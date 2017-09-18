import zmq
import time
import sys
import random

MINUTES = 60
###############################################################################
clients = ['brazil','london','california','mumbai','sydney','seoul','virginia']
NUM_ORDERINGS = 20
EXP_BLOCK_DURATION = 10 * MINUTES
INITIAL_WAIT_TIME = 2 * MINUTES
###############################################################################

def current_time():
    return int(time.time())

def create_next_ordering(clients, most_recent_client):
    ordering = random.sample(clients, len(clients))
    if ordering[0] == most_recent_client:
        swapi = random.randint(0,len(clients)-1)
        tmp = ordering[0]
        ordering[0] = ordering[swapi]
        ordering[swapi] = tmp
    return ordering

schedule = []
most_recent_client = None
for i in range(NUM_ORDERINGS):
    schedule += create_next_ordering(clients, most_recent_client)
    most_recent_client = schedule[-1]

current_place = 0
next_increment_time = current_time() + INITIAL_WAIT_TIME

def should_increment():
    if current_time() >= next_increment_time:
        return True
    return False

def update():
    global next_increment_time
    global current_place
    if should_increment():
        next_increment_time = current_time() + EXP_BLOCK_DURATION
        current_place += 1

def time_until_next_client_slot(client):
    if current_place == 0:
        i = current_place
    else:
        i = current_place + 1
    i = current_place

    t = next_increment_time - current_time()
    while i < len(schedule):
        if schedule[i] == client:
            break
        t += EXP_BLOCK_DURATION
        i += 1
    assert((i - current_place)<= (2*len(clients)-1))
    return i, t

###############################################################################

def handle_msg(msg):
    req_from = msg
    if not req_from in clients:
        return '-1'
    update()
    slot, time_until_slot = time_until_next_client_slot(req_from)
    return str(time_until_slot)

def test():
    t = 0 
    for i in range(100):
        print t,":",schedule[current_place:current_place+len(clients)]
        for client in schedule[:len(clients)]:
            print client,handle_msg(client)
        t+=1
        time.sleep(1)

###############################################################################

SERVER_PORT = 4052
context = zmq.Context()
socket = context.socket(zmq.REP)
socket.bind("tcp://*:%s" % SERVER_PORT)

while True:
    msg = socket.recv()
    resp = handle_msg(msg)
    socket.send(resp)

