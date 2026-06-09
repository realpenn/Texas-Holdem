package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"texas-holdem/internal/poker"
)

func (s *Store) CreateWaitingGame(ctx context.Context, game *poker.Game, deadline time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.assertNoActiveGameTx(ctx, tx, game.ChatID); err != nil {
		return err
	}
	if err := s.insertOrUpdateGameTx(ctx, tx, game, 0, deadline); err != nil {
		return err
	}
	if err := s.replacePlayersTx(ctx, tx, game); err != nil {
		return err
	}
	if err := s.eventTx(ctx, tx, game.ID, game.ChatID, game.CreatorID, "game_created", "{}"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveWaitingGame(ctx context.Context, game *poker.Game, waitingMessageID int, deadline time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.insertOrUpdateGameTx(ctx, tx, game, waitingMessageID, deadline); err != nil {
		return err
	}
	if err := s.replacePlayersTx(ctx, tx, game); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) BeginGame(ctx context.Context, game *poker.Game, deadline time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range game.Players {
		if _, err := s.adjustBalanceTx(ctx, tx, game.ChatID, p.UserID, game.ID, "buy_in", -game.BuyIn, "牌局买入"); err != nil {
			return err
		}
	}
	if err := s.insertOrUpdateGameTx(ctx, tx, game, 0, deadline); err != nil {
		return err
	}
	if err := s.replacePlayersTx(ctx, tx, game); err != nil {
		return err
	}
	if err := s.eventTx(ctx, tx, game.ID, game.ChatID, game.CreatorID, "game_started", "{}"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveRunningGame(ctx context.Context, game *poker.Game, deadline time.Time, actorID int64, kind string, payload string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.insertOrUpdateGameTx(ctx, tx, game, 0, deadline); err != nil {
		return err
	}
	if err := s.replacePlayersTx(ctx, tx, game); err != nil {
		return err
	}
	if kind != "" {
		if err := s.eventTx(ctx, tx, game.ID, game.ChatID, actorID, kind, payload); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) FinishGame(ctx context.Context, game *poker.Game) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, award := range game.Awards {
		if award.Net > 0 {
			if _, err := s.adjustBalanceTx(ctx, tx, game.ChatID, award.UserID, game.ID, "payout", award.Net, award.Reason); err != nil {
				return err
			}
		}
		if award.Fee > 0 {
			if err := s.insertFeeTx(ctx, tx, game.ChatID, game.ID, award.Fee, "服务费"); err != nil {
				return err
			}
		}
	}
	if err := s.insertOrUpdateGameTx(ctx, tx, game, 0, time.Time{}); err != nil {
		return err
	}
	if err := s.replacePlayersTx(ctx, tx, game); err != nil {
		return err
	}
	if err := s.eventTx(ctx, tx, game.ID, game.ChatID, 0, "game_finished", "{}"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CancelWaitingGame(ctx context.Context, game *poker.Game, userID int64) error {
	if game.Status != poker.StatusWaiting {
		return errors.New("只能取消等待中的牌局")
	}
	game.Status = poker.StatusCanceled
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.insertOrUpdateGameTx(ctx, tx, game, 0, time.Time{}); err != nil {
		return err
	}
	if err := s.eventTx(ctx, tx, game.ID, game.ChatID, userID, "game_canceled", "{}"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ActiveGame(ctx context.Context, chatID int64) (ActiveGame, error) {
	var state string
	var waitingMessageID int
	var deadline sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT state_json, waiting_message_id, action_deadline
FROM games
WHERE chat_id = ? AND status IN ('waiting', 'running')
ORDER BY created_at DESC
LIMIT 1`, chatID).Scan(&state, &waitingMessageID, &deadline)
	if errors.Is(err, sql.ErrNoRows) {
		return ActiveGame{}, ErrNotFound
	}
	if err != nil {
		return ActiveGame{}, err
	}
	var game poker.Game
	if err := json.Unmarshal([]byte(state), &game); err != nil {
		return ActiveGame{}, err
	}
	out := ActiveGame{Game: &game, WaitingMessageID: waitingMessageID}
	if deadline.Valid && deadline.String != "" {
		t, err := time.Parse(time.RFC3339, deadline.String)
		if err == nil {
			out.ActionDeadline = t
		}
	}
	return out, nil
}

func (s *Store) RunningGames(ctx context.Context) ([]ActiveGame, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT state_json, action_deadline
FROM games
WHERE status = 'running'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveGame
	for rows.Next() {
		var state string
		var deadline sql.NullString
		if err := rows.Scan(&state, &deadline); err != nil {
			return nil, err
		}
		var game poker.Game
		if err := json.Unmarshal([]byte(state), &game); err != nil {
			return nil, err
		}
		item := ActiveGame{Game: &game}
		if deadline.Valid && deadline.String != "" {
			t, err := time.Parse(time.RFC3339, deadline.String)
			if err == nil {
				item.ActionDeadline = t
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) WaitingGamesDue(ctx context.Context, now time.Time) ([]ActiveGame, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT state_json, waiting_message_id, action_deadline
FROM games
WHERE status = 'waiting' AND action_deadline IS NOT NULL AND action_deadline <= ?`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveGame
	for rows.Next() {
		var state string
		var waitingMessageID int
		var deadline sql.NullString
		if err := rows.Scan(&state, &waitingMessageID, &deadline); err != nil {
			return nil, err
		}
		var game poker.Game
		if err := json.Unmarshal([]byte(state), &game); err != nil {
			return nil, err
		}
		item := ActiveGame{Game: &game, WaitingMessageID: waitingMessageID}
		if deadline.Valid && deadline.String != "" {
			t, err := time.Parse(time.RFC3339, deadline.String)
			if err == nil {
				item.ActionDeadline = t
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) assertNoActiveGameTx(ctx context.Context, tx *sql.Tx, chatID int64) error {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM games WHERE chat_id = ? AND status IN ('waiting', 'running') LIMIT 1`, chatID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("本群已有等待中或进行中的牌局")
}

func (s *Store) insertOrUpdateGameTx(ctx context.Context, tx *sql.Tx, game *poker.Game, waitingMessageID int, deadline time.Time) error {
	state, err := json.Marshal(game)
	if err != nil {
		return err
	}
	var deadlineValue any
	if !deadline.IsZero() {
		deadlineValue = deadline.UTC().Format(time.RFC3339)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO games(id, chat_id, creator_id, status, small_blind, big_blind, buy_in, wait_seconds, action_seconds, rake_percent, rake_cap, state_json, waiting_message_id, action_deadline, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
  status = excluded.status,
  state_json = excluded.state_json,
  waiting_message_id = CASE WHEN excluded.waiting_message_id = 0 THEN games.waiting_message_id ELSE excluded.waiting_message_id END,
  action_deadline = excluded.action_deadline,
  updated_at = CURRENT_TIMESTAMP`,
		game.ID, game.ChatID, game.CreatorID, game.Status, game.SmallBlind, game.BigBlind, game.BuyIn,
		game.WaitSeconds, game.ActionSeconds, game.RakePercent, game.RakeCap, string(state), waitingMessageID, deadlineValue)
	return err
}

func (s *Store) replacePlayersTx(ctx context.Context, tx *sql.Tx, game *poker.Game) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM game_players WHERE game_id = ?`, game.ID); err != nil {
		return err
	}
	for _, p := range game.Players {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO game_players(game_id, chat_id, user_id, seat, display_name, stack, status)
VALUES (?, ?, ?, ?, ?, ?, ?)`, game.ID, game.ChatID, p.UserID, p.Seat, p.Display, p.Stack, p.Status); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) eventTx(ctx context.Context, tx *sql.Tx, gameID string, chatID, userID int64, kind string, payload string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO game_events(game_id, chat_id, user_id, kind, payload_json)
VALUES (?, ?, ?, ?, ?)`, gameID, chatID, userID, kind, payload)
	return err
}
