from __future__ import print_function
# Adapted from https://github.com/benthor/remotty/blob/master/socketclient.py

from select import select
import sys
import tty
import fcntl
import os
import termios
import threading
import errno

import logging
log = logging.getLogger(__name__)


class SocketClient:
    def __init__(self,
        socket_in=None,
        socket_out=None,
        socket_err=None,
        raw=True,
    ):
        self.socket_in = socket_in
        self.socket_out = socket_out
        self.socket_err = socket_err
        self.raw = raw

        self.stdin_fileno = sys.stdin.fileno()

    def __enter__(self):
        self.create()
        return self

    def __exit__(self, type, value, trace):
        self.destroy()

    def create(self):
        if os.isatty(sys.stdin.fileno()):
            self.settings = termios.tcgetattr(sys.stdin.fileno())
        else:
            self.settings = None

        if self.socket_in is not None:
            self.set_blocking(sys.stdin, False)
            self.set_blocking(sys.stdout, True)
            self.set_blocking(sys.stderr, True)

        if self.raw:
            tty.setraw(sys.stdin.fileno())

    def set_blocking(self, file, blocking):
        fd = file.fileno()
        flags = fcntl.fcntl(fd, fcntl.F_GETFL)
        flags = (flags & ~os.O_NONBLOCK) if blocking else (flags | os.O_NONBLOCK)
        fcntl.fcntl(fd, fcntl.F_SETFL, flags)

    def run(self):
        if self.socket_in is not None:
            self.start_background_thread(target=self.send_ws, args=(self.socket_in, sys.stdin))

        recv_threads = []

        if self.socket_out is not None:
            recv_threads.append(self.start_background_thread(target=self.recv_ws, args=(self.socket_out, sys.stdout)))

        if self.socket_err is not None:
            recv_threads.append(self.start_background_thread(target=self.recv_ws, args=(self.socket_err, sys.stderr)))

        for t in recv_threads:
            t.join()

    def start_background_thread(self, **kwargs):
        thread = threading.Thread(**kwargs)
        thread.daemon = True
        thread.start()
        return thread

    def recv_ws(self, socket, stream):
        try:
            while True:
                chunk = socket.recv()

                if chunk:
                    stream.write(chunk)
                    stream.flush()
                else:
                    break
        except Exception as e:
            log.debug(e)

    def send_ws(self, socket, stream):
        while True:
            r, w, e = select([stream.fileno()], [], [])

            if r:
                chunk = stream.read(1)

                if chunk == '':
                    socket.send_close()
                    break
                else:
                    try:
                        socket.send(chunk)
                    except Exception as e:
                        if hasattr(e, 'errno') and e.errno == errno.EPIPE:
                            break
                        else:
                            raise e

    def destroy(self):
        if self.settings is not None:
            termios.tcsetattr(self.stdin_fileno, termios.TCSADRAIN, self.settings)

        sys.stdout.flush()

if __name__ == '__main__':
    import websocket

    if len(sys.argv) != 2:
        sys.stderr.write("Usage: python socketclient.py WEBSOCKET_URL\n")
        exit(1)

    url = sys.argv[1]
    socket = websocket.create_connection(url)

    print("connected\r")

    with SocketClient(socket, interactive=True) as client:
        client.run()
