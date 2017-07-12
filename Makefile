all: cc-client cc-server

cc-client: client/main.go
	go build -o ./cc-client ./client/main.go

cc-server: server/main.go
	go build -o ./cc-server ./server/main.go

clean:
	rm -f ./cc-client
	rm -f ./cc-server
