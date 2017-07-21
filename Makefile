all: cc-client cc-server db

cc-client: client/*.go config/*.go results/*.go
	go build -o ./cc-client ./client/

cc-server: server/*.go config/*.go results/*.go
	go build -o ./cc-server ./server/

db: db/*.go config/*.go results/*.go
	go build -o ./db-server ./db/

clean:
	rm -f ./cc-client
	rm -f ./cc-server
