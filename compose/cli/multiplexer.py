from __future__ import absolute_import
from threading import Thread

try:
    from Queue import Queue, Empty
except ImportError:
    from queue import Queue, Empty  # Python 3.x


STOP = object()


class Multiplexer(object):
    """
    Create a single iterator from several iterators by running all of them in
    parallel and yielding results as they come in.
    """

    def __init__(self, iterators):
        self.iterators = iterators
        self._num_running = len(iterators)
        self.queue = Queue()

    def loop(self):
        self._init_readers()

        while self._num_running > 0:
            try:
                item, exception = self.queue.get(timeout=0.1)

                if exception:
                    raise exception

                if item is STOP:
                    self._num_running -= 1
                else:
                    yield item
            except Empty:
                pass

    def _init_readers(self):
        for iterator in self.iterators:
            t = Thread(target=_enqueue_output, args=(iterator, self.queue))
            t.daemon = True
            t.start()


def _enqueue_output(iterator, queue):
    try:
        for item in iterator:
            queue.put((item, None))
        queue.put((STOP, None))
    except Exception as e:
        queue.put((None, e))
