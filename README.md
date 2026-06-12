# Telegram Texas Hold'em Bot

群内娱乐积分德州扑克 bot。第一版是单机 SQLite 部署，每个群同一时间只允许一局现金桌 NLH。

## 快速开始

```bash
cp .env.example .env
# 填写 BOT_TOKEN、ADMIN_USER_IDS、ALLOWED_CHAT_IDS
go run ./cmd/bot
```

## 主要命令

- `/newgame [sb] [bb] [buyin] [wait_seconds]` 创建牌桌
- `/join` 买入加入（等待中或两手之间均可）
- `/leave` 离桌，剩余筹码结算回余额（等待中或两手之间）
- `/begin` 发起人提前开局
- `/cancel` 发起人或管理员取消等待局；牌局进行中在两手之间可关闭牌桌
- `/balance` 查询余额
- `/checkin` 每日签到
- `/redeem CODE` 兑换充值码

牌局行动通过群内按钮完成，加注有最小加注/半池/满池/All-in 档位。

牌桌是连续现金桌：开局随机定庄，之后逐手轮转；一手结束后短暂停顿自动开始下一手，
期间可离桌/买入加入；筹码输光自动离桌，剩余不足 2 人时自动关桌。买入在开局或入桌时
从余额扣除，离桌或关桌时按剩余筹码退回；抽水按手结算，未被跟注的下注不抽水。

21 点玩法已拆分至独立仓库 [realpenn/Blackjack](https://github.com/realpenn/Blackjack)。

充值码只按总次数限制；只要次数没用完，同一用户可以重复兑换同一个充值码。

管理员私聊命令：

- `/code CODE AMOUNT MAX_USES [YYYY-MM-DD]` 创建充值码
- `/voidcode CODE` 作废充值码
- `/codeinfo CODE` 查询充值码

## 说明

本 bot 只记录娱乐积分，不支持真钱充值、提现、支付或任何现实价值兑换。
