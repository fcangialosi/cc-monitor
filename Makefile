all: cc-client cc-server db-server

cc-client: client/*.go config/*.go results/*.go
	go build -o ./cc-client ./client/

cc-server: server/*.go config/*.go results/*.go
	go build -o ./cc-server ./server/

db-server: db/*.go config/*.go results/*.go
	go build -o ./db-server ./db/

clean:
	rm -f ./cc-client
	rm -f ./cc-server
	rm -f ./db-server
