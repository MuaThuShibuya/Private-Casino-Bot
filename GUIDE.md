# 🎰 Discord Casino V2 — Hướng Dẫn Sử Dụng & Tổng Quan

---

## MỤC LỤC
1. [Tổng quan hệ thống](#1-tổng-quan-hệ-thống)
2. [Cài đặt & Khởi chạy](#2-cài-đặt--khởi-chạy)
3. [Biến môi trường (ENV)](#3-biến-môi-trường-env)
4. [Toàn bộ lệnh bot](#4-toàn-bộ-lệnh-bot)
5. [Luật game chi tiết](#5-luật-game-chi-tiết)
6. [Hệ thống phân quyền](#6-hệ-thống-phân-quyền)
7. [Tuỳ chỉnh Emoji & Icon](#7-tuỳ-chỉnh-emoji--icon)
8. [Kiến trúc bảo mật](#8-kiến-trúc-bảo-mật)
9. [Deploy lên Render](#9-deploy-lên-render)

---

## 1. Tổng quan hệ thống

```
Discord Casino V2
├── Nền tảng     : Golang 1.21 + discordgo v0.27.1
├── Database     : MongoDB Atlas (cluster miễn phí được)
├── Deploy       : Render (Web Service)
├── Prefix lệnh  : ! (ví dụ: !taixiu, !bj, !pay)
├── Slash lệnh   : /balance, /transfer, /history, /admin
└── Bảo mật tiền : MongoDB Atomic Transaction — không thể hack số dư âm
```

### Sơ đồ luồng tiền trong game
```
[Player gõ !taixiu 1000]
      ↓
[Bot kiểm tra balance ≥ 1000]
      ↓
[wallet.Withdraw() — trừ ngay, atomic $inc + $gte filter]
      ↓
[Ghi vào danh sách cược]
      ↓
[30s sau → tung xúc xắc bằng crypto/rand]
      ↓
[Người thắng → wallet.Deposit() × 2]   [Người thua → đã trừ rồi, ghi log]
      ↓
[TransactionLog ghi đầy đủ để đối soát]
```

---

## 2. Cài đặt & Khởi chạy

### Yêu cầu
- Go 1.21+
- MongoDB Atlas account (free tier M0 là đủ)
- Discord Developer Portal — tạo Bot application

### Bước 1 — Tải dependencies
```bash
cd "Discord Casino V2"
go mod tidy
```

### Bước 2 — Tạo file .env
```bash
cp .env.example .env
# Điền đầy đủ các giá trị vào .env
```

### Bước 3 — Thiết lập MongoDB Atlas
1. Tạo cluster → Database → Collection tự động tạo khi bot chạy lần đầu
2. Network Access → Allow `0.0.0.0/0` (cho Render)
3. Database Access → Tạo user với quyền `readWrite`
4. Lấy connection string: `mongodb+srv://user:pass@cluster.mongodb.net/`

### Bước 4 — Discord Bot Setup
1. [Discord Developer Portal](https://discord.com/developers/applications) → New Application
2. Bot → Add Bot → Copy Token → điền vào `DISCORD_TOKEN`
3. **Privileged Gateway Intents** → BẬT:
   - `SERVER MEMBERS INTENT`
   - `MESSAGE CONTENT INTENT` ← **BẮT BUỘC** để nhận lệnh `!`
4. OAuth2 → URL Generator → Scopes: `bot`, `applications.commands`
5. Bot Permissions: `Send Messages`, `Embed Links`, `Read Message History`, `Use External Emojis`

### Bước 5 — Chạy local
```bash
go run ./cmd/bot/
```

---

## 3. Biến môi trường (ENV)

| Biến | Bắt buộc | Mô tả |
|------|----------|-------|
| `DISCORD_TOKEN`| Token bot từ Discord Developer Portal |
| `MONGODB_URI`| Connection string MongoDB Atlas |
| `DB_NAME`| Tên database (mặc định: `casino`) |
| `PORT`| Port HTTP keep-alive (Render tự set, mặc định: `8080`) |
| `GUILD_ID`| Xem giải thích bên dưới |

### GUILD_ID — Có cần thiết không?

**Ngắn gọn: Không cần thiết cho production. Chỉ dùng khi dev.**

Giải thích chi tiết:
- `GUILD_ID` chỉ ảnh hưởng đến **Slash Commands** (`/balance`, `/transfer`, `/history`, `/admin`)
- **Tất cả game** (`!taixiu`, `!bj`) dùng prefix `!` → **không liên quan đến GUILD_ID**

| Trường hợp | GUILD_ID | Kết quả |
|------------|----------|---------|
| Dev/test   | Điền ID server | Slash commands active ngay lập tức |
| Production | Để trống `""` | Slash commands mất ~1 giờ để propagate toàn Discord |
| Chỉ dùng `!` commands | Bỏ qua hoàn toàn | Slash commands không register |

**Khuyến nghị:** Để trống `GUILD_ID` khi deploy Render. Slash commands sẽ propagate sau 1 tiếng. Các lệnh game `!` không bị ảnh hưởng.

---

## 4. Toàn bộ lệnh bot

### 🎮 Lệnh Game (Prefix `!`)

| Lệnh | Cú pháp | Mô tả |
|------|---------|-------|
| Tài Xỉu | `!taixiu [thời gian] [min] [max]` | Mở ván cược tập thể |
| Bầu Cua | `!baucua [thời gian] [min] [max]` | Mở ván cược Bầu Cua |
| Xì Dách | `!xidach [thời gian] [số tiền]` | Chơi Xì Dách với nhà cái |

Alias hỗ trợ:
```
!taixiu = !tx
!blackjack = !bj
```

### 💰 Lệnh Ví (Prefix `!`)

| Lệnh | Cú pháp | Mô tả |
|------|---------|-------|
| Chuyển tiền | `!pay @user [số tiền]` | Chuyển tiền theo @mention |
| Chuyển tiền | `!pay [discord.com/users/ID] [số tiền]` | Chuyển theo link profile |
| Chuyển tiền | `!pay [ID thuần] [số tiền]` | Chuyển theo raw ID |

Ví dụ:
```
!pay @Minh 5000
!pay https://discord.com/users/123456789012345678 5000
!pay 123456789012345678 5000
```

### ⚙️ Lệnh Admin (Prefix `!`)

| Lệnh | Quyền | Mô tả |
|------|-------|-------|
| `!admin @user` | Owner | Cấp quyền Admin cho user |
| `!admin remove @user` | Owner | Thu hồi quyền Admin |
| `!limit on` | Owner | Bật giới hạn cược |
| `!limit off` | Owner | Tắt giới hạn cược |
| `!whitelist add #kênh` | Owner | Cho phép kênh dùng casino |
| `!whitelist remove #kênh` | Owner | Cấm kênh dùng casino |

### 📊 Slash Commands (`/`)

| Lệnh | Mô tả |
|------|--------------------------------------------------------------------|
| `/balance [@user]` | Xem bảng xếp hạng phú hộ. Nếu có `@user`, xem số dư của người đó. |
| `/transfer @user [số tiền]` | Chuyển tiền |
| `/history [số lượng]` | Lịch sử giao dịch gần nhất |
| `/admin give @user [số tiền]` | Admin cộng tiền (cần quyền Admin) |
| `/admin take @user [số tiền]` | Admin trừ tiền (cần quyền Admin) |
| `/admin setrole @user [role]` | Đặt vai trò (cần quyền Owner) |
| `/admin whitelist add/remove #kênh` | Quản lý whitelist (cần quyền Owner) |

---

## 5. Luật game chi tiết

### 🎲 Tài Xỉu
```
Tung 3 xúc xắc (dùng crypto/rand — không thể can thiệp)
  Tổng 3–10  → Xỉu (Small)
  Tổng 11–18 → Tài (Big)
  Tam Giống  → Nhà cái thắng TẤT (không hoàn tiền)

Tỉ lệ: Thắng nhận 2× số tiền cược
Thời gian: Tuỳ chỉnh khi gọi lệnh — tự động giải quyết hoặc nhấn Bắt Đầu để tung xúc xắc ngay
Nhiều người: Mỗi người nhấn nút bấm với cùng mức cược
Mỗi người chỉ cược được 1 lần/ván
```

### 🃏 Blackjack
```
Luật chuẩn quốc tế:
  Ace = 1 hoặc 11 (tự động chọn tốt nhất)
  J, Q, K = 10
  Blackjack (21 ngay 2 lá) = thắng 3:2 (nếu nhà cái không cũng BJ)
  Dealer: rút đến ≥17 thì dừng

Các lựa chọn:
  [Rút Thêm / Hit]    — Rút thêm 1 lá
  [Dừng / Stand]      — Nhường lượt cho dealer
  [Nhân Đôi / Double] — Nhân đôi tiền cược, rút đúng 1 lá rồi đứng

Bảo mật: Nút bấm khoá theo người chơi — người khác nhấn bị từ chối
```

### 🚌 Ride The Bus (4 Giai Đoạn)
```
Mỗi giai đoạn đúng → nhân thêm phần thưởng tiềm năng
Sai bất kỳ giai đoạn → mất toàn bộ tiền cược

Giai đoạn 1 — Đỏ hay Đen?       Thưởng nếu đúng:  2× cược
Giai đoạn 2 — Cao hay Thấp?      Thưởng nếu đúng:  3× cược
Giai đoạn 3 — Trong hay Ngoài?   Thưởng nếu đúng:  5× cược
Giai đoạn 4 — Chất Bài?          Thưởng nếu đúng: 10× cược

Lưu ý:
  - Bằng nhau ở giai đoạn 2 = Sai (strict)
  - Chạm biên ở giai đoạn 3 = Sai (strict)
```

### 🪙 Đoán Xu
```
Xác suất 50/50 — không thể gian lận (crypto/rand)
  Thắng → nhận 2× tiền cược
  Thua  → mất tiền cược
```

---

## 6. Hệ thống phân quyền

```
Owner  ──── Toàn quyền (cấp Admin, manage whitelist, giới hạn cược)
  │
  └── Admin  ── Cộng/trừ tiền người chơi, xem lịch sử
         │
         └── User  ── Chơi game, chuyển tiền, xem số dư
```

### Thiết lập Owner đầu tiên
Owner **không thể tự cấp qua lệnh** — phải insert trực tiếp vào MongoDB:

```javascript
// MongoDB Atlas → Browse Collections → users → Insert Document
{
  "_id": "DISCORD_USER_ID_CỦA_BẠN",
  "balance": 0,
  "role": "owner",
  "created_at": new Date(),
  "updated_at": new Date()
}
```

Sau khi có Owner, dùng `!admin @user` để cấp Admin cho người khác.

### Whitelist Channel
Mặc định khi chưa có channel nào trong whitelist → **tất cả kênh đều được dùng**.

Sau khi add ít nhất 1 channel vào whitelist → **chỉ các kênh đó được dùng**.

```
!whitelist add #casino-room    → Chỉ cho phép #casino-room
!whitelist add #vip-room       → Thêm #vip-room vào danh sách
!whitelist remove #casino-room → Xóa #casino-room
```

---

## 7. Tuỳ chỉnh Emoji & Icon

### File quản lý icon
```
internal/games/emoji.go
```

Tất cả icon được nhóm thành 7 section:

| Section | Mô tả |
|---------|-------|
| 1. Tiền tệ & Kinh tế | `EmojiCoin`, `EmojiWin`, `EmojiLose`… |
| 2. Hệ thống & Thông báo | `EmojiSuccess`, `EmojiError`, `EmojiTimer`… |
| 3. Phân quyền (Role) | `EmojiCrown`, `EmojiSword`, `EmojiBlock`… |
| 4. Tài Xỉu | `EmojiTai`, `EmojiXiu`, `EmojiTriple`, `DiceFaces[]` |
| 5. Blackjack | `EmojiCard`, `EmojiBust`, `EmojiHit`, `SuitEmoji[]` |
| 6. Ride The Bus | `EmojiRed`, `EmojiBlack`, `EmojiUp`, `EmojiDown`… |
| 7. Đoán Xu | `EmojiHeads`, `EmojiTails`, `EmojiFlip` |

### Cách thay bằng emoji server

**Bước 1** — Lấy ID emoji:
```
Trong Discord: gõ  \:tên_emoji:  → bot sẽ hiện ra  <:tên:ID>
Hoặc: Server Settings → Emoji → chuột phải → Copy ID
```

**Bước 2** — Mở `internal/games/emoji.go`, thay giá trị:
```go
// Trước (emoji mặc định):
EmojiCoin = "🪙"

// Sau (emoji server của bạn):
EmojiCoin = "<:mycoin:1234567890123456>"

// Animated emoji:
EmojiWin = "<a:fireworks:1234567890123456>"
```

**Bước 3** — Rebuild:
```bash
go build ./cmd/bot/
```

### Ví dụ thực tế
```go
// Section 4 — Tài Xỉu sau khi custom:
EmojiTai    = "<:tai:1111111111111111>"   // icon lửa đỏ
EmojiXiu    = "<:xiu:2222222222222222>"   // icon băng xanh
EmojiTriple = "<a:skull:3333333333333333>" // skull animated
EmojiDice   = "<a:dice:4444444444444444>"  // xúc xắc quay

// Mặt xúc xắc:
var DiceFaces = [7]string{"",
    "<:d1:1000>", "<:d2:1001>", "<:d3:1002>",
    "<:d4:1003>", "<:d5:1004>", "<:d6:1005>",
}
```

> **Lưu ý:** Bot phải ở trong server chứa emoji đó, hoặc emoji phải là Nitro emoji.

---

## 8. Kiến trúc bảo mật

### Chống Race Condition (không dùng Redis)
```
Cách KHÔNG AN TOÀN (check-then-act):
  1. Đọc balance = 100
  2. Kiểm tra 100 >= 50 → OK
  3. Trừ 50
  ↑ Nếu 2 request đồng thời thì cả 2 đều "thấy" 100 → trừ 2 lần

Cách AN TOÀN (MongoDB atomic):
  filter: { _id: userID, balance: { $gte: 50 } }
  update: { $inc: { balance: -50 } }
  → MongoDB tự lock document khi update
  → Nếu balance không đủ → matchedCount = 0 → trả lỗi
  → KHÔNG THỂ xảy ra số dư âm
```

### Game Lock (sync.Map)
```
Khi user bắt đầu Blackjack/RTB/ĐoánXu:
  LockUser(userID) → lưu vào sync.Map
  ↓
  Nếu user thử bắt đầu game khác → bị từ chối
  ↓
  Khi game kết thúc (win/lose/bust) → UnlockUser(userID)
```

### Transfer — MongoDB Session Transaction
```
Giao dịch chuyển tiền dùng Saga đơn giản:
  session.StartTransaction()
  → Step 1: Trừ người gửi (có $gte guard)
  → Step 2: Cộng người nhận
  → Step 3: Ghi 2 dòng TransactionLog
  session.CommitTransaction()

  Nếu bất kỳ bước nào lỗi → session.AbortTransaction()
  → Không mất tiền, không lỗi dữ liệu
```

### Rate Limiter
```
Mỗi user chỉ được gửi 1 lệnh mỗi giây
  Allow(userID) → kiểm tra last_seen
  Nếu quá nhanh → gửi cảnh báo (tự xóa sau 3s)
```

### Interaction Lock (Blackjack/RTB)
```
Button customID nhúng playerID: "bj:hit:123456789"
  → Handler check: interactionUserID(i) == ownerID
  → Không khớp → ephemeral "Đây không phải ván của bạn"
  → Người khác KHÔNG thể can thiệp vào ván của người khác
```

---

## 9. Deploy lên Render

### Bước 1 — Tạo Web Service trên Render
1. [render.com](https://render.com) → New → Web Service
2. Connect GitHub repo
3. Build Command: `go build -o bot ./cmd/bot/`
4. Start Command: `./bot`
5. Environment: **Go**

### Bước 2 — Điền Environment Variables
Trong Render Dashboard → Environment:
```
DISCORD_TOKEN  = [bot token]
MONGODB_URI    = [atlas connection string]
DB_NAME        = casino
GUILD_ID       = (để trống)
```
> Render tự set `PORT` — không cần điền.

### Bước 3 — Keep-Alive (đã tích hợp sẵn)
Bot đã có HTTP server trả `200 OK` trên `$PORT`.
Render sẽ ping endpoint này để không bị spin down.

Nếu dùng **Free tier** (có thể bị spin down sau 15 phút idle):
→ Dùng [UptimeRobot](https://uptimerobot.com) ping `https://your-app.onrender.com/health` mỗi 5 phút.

### Health Check Endpoints
```
GET /         → 200 OK  (Render uses this)
GET /health   → 200 {"status":"ok","ts":"2024-..."}
```

---

## Cấu trúc thư mục đầy đủ

```
Discord Casino V2/
├── cmd/
│   └── bot/
│       └── main.go              ← Entrypoint: HTTP keep-alive + Discord init
├── internal/
│   ├── models/
│   │   ├── user.go              ← Schema: User (ID, Balance, Role)
│   │   ├── config.go            ← Schema: GlobalConfig (whitelist, limits)
│   │   └── transaction.go       ← Schema: TransactionLog (audit trail)
│   ├── repository/
│   │   ├── db.go                ← MongoDB connection + index creation
│   │   ├── wallet.go            ← Atomic deposit/withdraw/transfer
│   │   ├── user.go              ← User CRUD + role management
│   │   └── config.go            ← Whitelist + config management
│   ├── service/
│   │   └── wallet_service.go    ← Business logic layer
│   ├── games/
│   │   ├── emoji.go             ← ★ TẤT CẢ ICON — chỉnh sửa ở đây
│   │   ├── rng.go               ← crypto/rand wrapper
│   │   ├── taixiu.go            ← Logic Tài/Xỉu/Triple
│   │   ├── blackjack.go         ← Card/hand logic
│   │   ├── ridethebus.go        ← 4-stage RTB logic
│   │   └── doanxu.go            ← Coin flip logic
│   └── discord/
│       ├── bot.go               ← Bot init + interaction router
│       ├── middleware.go         ← RequireOwner/Admin/Channel + game lock
│       ├── ratelimiter.go       ← 1 cmd/s per user
│       ├── util.go              ← sendTemp, deleteAfter, coinDisplay
│       ├── prefix.go            ← ! command router + ID regex extractor
│       ├── cmd_admin.go         ← !admin, !limit, !whitelist
│       ├── cmd_wallet.go        ← !pay
│       ├── handler_taixiu.go    ← Multi-player 30s betting round
│       ├── handler_blackjack.go ← Hit/Stand/Double + interaction lock
│       ├── handler_rtb.go       ← 4-stage progressive game
│       └── handler_doanxu.go    ← Instant coin flip
├── go.mod
├── .env.example
└── GUIDE.md                     ← File này
```
