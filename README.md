# Telegram Texas Hold'em Bot

群内娱乐积分德州扑克 bot。第一版是单机 SQLite 部署，每个群同一时间只允许一局现金桌 NLH。

## 快速开始

```bash
cp .env.example .env
# 填写 BOT_TOKEN、ADMIN_USER_IDS、ALLOWED_CHAT_IDS
go run ./cmd/bot
```

## 主要命令

- `/newgame [sb] [bb] [buyin] [wait_seconds]` 创建牌局
- `/join` 加入等待局
- `/leave` 离开等待局
- `/begin` 发起人提前开局
- `/cancel` 发起人或管理员取消等待局
- `/balance` 查询余额
- `/checkin` 每日签到
- `/redeem CODE` 兑换充值码

牌局行动通过群内按钮完成。

管理员私聊命令：

- `/code CODE AMOUNT MAX_USES [YYYY-MM-DD]` 创建充值码
- `/voidcode CODE` 作废充值码
- `/codeinfo CODE` 查询充值码

## 说明

本 bot 只记录娱乐积分，不支持真钱充值、提现、支付或任何现实价值兑换。
