import zmq
import sys

SERVER_IP = 'localhost'
SERVER_PORT = 4052
ME = sys.argv[1] # TODO MAKE SURE TO PUT NAME STRING HERE

context = zmq.Context()
socket = context.socket(zmq.REQ)
socket.connect("tcp://%s:%s" % (SERVER_IP,SERVER_PORT))

socket.send(ME)
resp = socket.recv()
print int(resp)
