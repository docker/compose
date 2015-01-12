from __future__ import absolute_import
from threading import Thread

try:
    from Queue import Queue, Empty
except ImportError:
    from queue import Queue, Empty  # Python 3.x


# Yield STOP from an input generator to stop the
# top-level loop without processing any more input.
STOP = object()


class Multiplexer(object):
    def __init__(self, generators):
        self.generators = generators
        self.queue = Queue()

    def loop(self):
        self._init_readers()

        while True:
            try:
                item = self.queue.get(timeout=0.1)
                if item is STOP:
                    break
                else:
                    yield item
            except Empty:
                pass

    def _init_readers(self):
        for generator in self.generators:
            t = Thread(target=_enqueue_output, args=(generator, self.queue))
            t.daemon = True
            t.start()


def _enqueue_output(generator, queue):
    for item in generator:
        queue.put(item)
