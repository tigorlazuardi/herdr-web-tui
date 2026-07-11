.PHONY: build frontend test clean run

BIN := herdr-web-tui

build: frontend
	go build -o $(BIN) ./cmd/herdr-web-tui

frontend:
	cd frontend && npm install && npm run build

test:
	go test ./...

run: build
	./$(BIN)

clean:
	rm -f $(BIN)
	rm -rf frontend/dist
