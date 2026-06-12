package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"
)

func (s *Store) UpsertUser(ctx context.Context, id int64, username, firstName, lastName string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users(telegram_user_id, username, first_name, last_name, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(telegram_user_id) DO UPDATE SET
  username = excluded.username,
  first_name = excluded.first_name,
  last_name = excluded.last_name,
  updated_at = CURRENT_TIMESTAMP`, id, username, firstName, lastName)
	return err
}

func (s *Store) UpsertGroup(ctx context.Context, id int64, title string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO groups(telegram_chat_id, title, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(telegram_chat_id) DO UPDATE SET title = excluded.title, updated_at = CURRENT_TIMESTAMP`, id, title)
	return err
}

func (s *Store) Balance(ctx context.Context, chatID, userID int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	balance, err := s.ensureBalanceTx(ctx, tx, chatID, userID)
	if err != nil {
		return 0, err
	}
	return balance, tx.Commit()
}

func (s *Store) Checkin(ctx context.Context, chatID, userID int64, now time.Time) (int64, int64, error) {
	localDate := now.Format("2006-01-02")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if _, err = s.ensureBalanceTx(ctx, tx, chatID, userID); err != nil {
		return 0, 0, err
	}
	amount, err := randomCheckinAmount(s.dailyCheckin)
	if err != nil {
		return 0, 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO checkins(chat_id, user_id, local_date, amount) VALUES (?, ?, ?, ?)`, chatID, userID, localDate, amount)
	if err != nil {
		return 0, 0, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		balance, _ := s.balanceTx(ctx, tx, chatID, userID)
		return balance, 0, errors.New("今天已经签到过了")
	}
	balance, err := s.adjustBalanceTx(ctx, tx, chatID, userID, "", "checkin", amount, "每日签到")
	if err != nil {
		return 0, 0, err
	}
	return balance, amount, tx.Commit()
}

func (s *Store) CreateRechargeCode(ctx context.Context, code string, amount int64, maxUses int, expiresAt *time.Time, createdBy int64) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO recharge_codes(code, amount, max_uses, expires_at, created_by, status)
VALUES (?, ?, ?, ?, ?, 'active')`, code, amount, maxUses, nullableTime(expiresAt), createdBy)
	return err
}

func (s *Store) VoidRechargeCode(ctx context.Context, code string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE recharge_codes SET status = 'void' WHERE code = ?`, code)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RechargeCode(ctx context.Context, code string) (RechargeCode, error) {
	var out RechargeCode
	var expires sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT code, amount, max_uses, used_count, expires_at, created_by, status
FROM recharge_codes WHERE code = ?`, code).Scan(&out.Code, &out.Amount, &out.MaxUses, &out.UsedCount, &expires, &out.CreatedBy, &out.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return RechargeCode{}, ErrNotFound
	}
	if err != nil {
		return RechargeCode{}, err
	}
	if expires.Valid && expires.String != "" {
		t, err := time.Parse(time.RFC3339, expires.String)
		if err == nil {
			out.ExpiresAt = &t
		}
	}
	return out, nil
}

func (s *Store) Redeem(ctx context.Context, chatID, userID int64, code string, now time.Time) (int64, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var amount int64
	var maxUses, usedCount int
	var status string
	var expires sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT amount, max_uses, used_count, expires_at, status
FROM recharge_codes WHERE code = ?`, code).Scan(&amount, &maxUses, &usedCount, &expires, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, err
	}
	if status != "active" {
		return 0, 0, errors.New("充值码已失效")
	}
	if usedCount >= maxUses {
		return 0, 0, errors.New("充值码次数已用完")
	}
	if expires.Valid && expires.String != "" {
		exp, err := time.Parse(time.RFC3339, expires.String)
		if err == nil && now.After(exp) {
			return 0, 0, errors.New("充值码已过期")
		}
	}
	if _, err = s.ensureBalanceTx(ctx, tx, chatID, userID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO redemptions(code, chat_id, user_id, amount) VALUES (?, ?, ?, ?)`, code, chatID, userID, amount); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE recharge_codes SET used_count = used_count + 1 WHERE code = ?`, code); err != nil {
		return 0, 0, err
	}
	balance, err := s.adjustBalanceTx(ctx, tx, chatID, userID, "", "redeem", amount, "兑换充值码 "+code)
	if err != nil {
		return 0, 0, err
	}
	return balance, amount, tx.Commit()
}

func (s *Store) ensureBalanceTx(ctx context.Context, tx *sql.Tx, chatID, userID int64) (int64, error) {
	balance, err := s.balanceTx(ctx, tx, chatID, userID)
	if err == nil {
		return balance, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO balances(chat_id, user_id, balance) VALUES (?, ?, ?)`, chatID, userID, s.initialBalance); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO ledger_entries(chat_id, user_id, kind, amount, balance_after, note)
VALUES (?, ?, 'initial', ?, ?, '初始娱乐积分')`, chatID, userID, s.initialBalance, s.initialBalance); err != nil {
		return 0, err
	}
	return s.initialBalance, nil
}

func (s *Store) balanceTx(ctx context.Context, tx *sql.Tx, chatID, userID int64) (int64, error) {
	var balance int64
	err := tx.QueryRowContext(ctx, `SELECT balance FROM balances WHERE chat_id = ? AND user_id = ?`, chatID, userID).Scan(&balance)
	return balance, err
}

func (s *Store) adjustBalanceTx(ctx context.Context, tx *sql.Tx, chatID, userID int64, gameID, kind string, amount int64, note string) (int64, error) {
	balance, err := s.ensureBalanceTx(ctx, tx, chatID, userID)
	if err != nil {
		return 0, err
	}
	next := balance + amount
	if next < 0 {
		return 0, fmt.Errorf("余额不足，需要 %d，当前 %d", -amount, balance)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE balances SET balance = ?, updated_at = CURRENT_TIMESTAMP WHERE chat_id = ? AND user_id = ?`, next, chatID, userID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO ledger_entries(chat_id, user_id, game_id, kind, amount, balance_after, note)
VALUES (?, ?, ?, ?, ?, ?, ?)`, chatID, userID, gameID, kind, amount, next, note); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *Store) insertFeeTx(ctx context.Context, tx *sql.Tx, chatID int64, gameID string, amount int64, note string) error {
	if amount <= 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO ledger_entries(chat_id, user_id, game_id, kind, amount, balance_after, note)
VALUES (?, 0, ?, 'rake_fee', ?, 0, ?)`, chatID, gameID, amount, note)
	return err
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func randomCheckinAmount(maxAmount int64) (int64, error) {
	const minAmount int64 = 100
	if maxAmount <= minAmount {
		return minAmount, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(maxAmount-minAmount+1))
	if err != nil {
		return 0, err
	}
	return minAmount + n.Int64(), nil
}
