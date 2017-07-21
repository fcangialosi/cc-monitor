all: cc-client cc-server

cc-client: client/*.go config/*.go results/*.go
	go build -o ./cc-client ./client/

cc-server: server/*.go config/*.go results/*.go
	go build -o ./cc-server ./server/

clean:
	rm -f ./cc-client
	rm -f ./cc-server
