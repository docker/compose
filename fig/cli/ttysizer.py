import sys
import os
import fcntl
import termios
import struct
import signal

class TTYSizer:
    """
    Keeps the pseudo-tty of a container sized the same as the real tty.

    Pseudo-TTYs need to be resized when a SIGWINCH signal is received on the
    real TTY, so that the number of rows and columns matches that of the host.
    """

    def __init__(self, container):
        """
        Initialize a TTYSizer that monitors `container`.

        When the TTYSizer is started, it will run in a sub-process so that the
        received signals can be processed immediately and are not blocked on
        the main process running the pseudo-tty.
        """
        self.container = container
        self.client = container.client

    def start(self):
        """
        Spawn the child process that will keep the pseudo-tty correctly sized.

        This method returns immediately in the main process.
        """
        if os.fork() > 0:
            return

        def handler(signum, frame):
            if signum == signal.SIGWINCH:
                self.resize_tty()

        signal.signal(signal.SIGWINCH, handler)

        self.resize_tty()
        self.container.wait()
        sys.exit(0)

    def resize_tty(self):
        """
        Set the size of the pseudo-tty in the container to match the real tty.
        """
        size = self._get_tty_size()

        if size is not None:
            h, w = size
            url = self.client._url(
                "/containers/{0}/resize".format(self.container.id)
            )

            self.client._post(url, params={'h': h, 'w': w})

    # http://blog.taz.net.au/2012/04/09/getting-the-terminal-size-in-python/
    def _get_tty_size(self):
        try:
            hw = struct.unpack(
                'hh',
                fcntl.ioctl(sys.stdout.fileno(), termios.TIOCGWINSZ, '1234')
            )
        except:
            try:
                hw = (os.environ['LINES'], os.environ['COLUMNS'])
            except:
                return

        return hw
