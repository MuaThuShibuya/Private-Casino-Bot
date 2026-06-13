## Discord Casino Bot — Lệnh build/run/test
## Yêu cầu: Go 1.21+, make (Linux/Mac/WSL)
## Windows không có make: dùng lệnh go trực tiếp bên dưới

# Tên binary output
ifeq ($(OS),Windows_NT)
    BINARY := bot.exe
else
    BINARY := bot
endif

.PHONY: run build test race clean help

# ─── Lệnh thường dùng ────────────────────────────────────────────────────────

## Chạy thẳng không cần compile (dev local)
run:
	go run ./cmd/bot

## Compile ra binary ./bot (hoặc bot.exe trên Windows)
build:
	go build -o $(BINARY) ./cmd/bot

## Chạy binary đã compile
start: build
	./$(BINARY)

# ─── Test ────────────────────────────────────────────────────────────────────

## Chạy toàn bộ unit test
test:
	go test ./... -v

## Chạy test có race detector — bắt buộc chạy trước khi deploy
race:
	go test ./... -race -v

## Chỉ test game logic
test-games:
	go test ./internal/games/... -v

## Chỉ test wallet (concurrent + race)
test-wallet:
	go test ./internal/service/... -race -v

# ─── Dọn dẹp ─────────────────────────────────────────────────────────────────

## Xóa binary đã compile
clean:
	go clean
	-rm -f bot bot.exe

# ─── Kiểm tra code ───────────────────────────────────────────────────────────

## Kiểm tra lỗi compile toàn bộ project
vet:
	go vet ./...

## Tidy go.mod và go.sum
tidy:
	go mod tidy

# ─── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "  Discord Casino Bot — Lệnh khả dụng:"
	@echo ""
	@echo "  make run          Chạy bot trực tiếp (không compile)"
	@echo "  make build        Compile ra binary ./bot"
	@echo "  make start        Compile rồi chạy"
	@echo ""
	@echo "  make test         Chạy toàn bộ unit test"
	@echo "  make race         Chạy test + race detector"
	@echo "  make test-games   Chỉ test game logic"
	@echo "  make test-wallet  Chỉ test wallet (concurrent)"
	@echo ""
	@echo "  make vet          Kiểm tra lỗi compile"
	@echo "  make tidy         Dọn go.mod"
	@echo "  make clean        Xóa binary"
	@echo ""

.DEFAULT_GOAL := help
