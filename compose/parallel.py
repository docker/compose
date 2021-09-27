import _thread as thread
import logging
import operator
import sys
from queue import Empty
from queue import Queue
from threading import Lock
from threading import Semaphore
from threading import Thread

from docker.errors import APIError
from docker.errors import ImageNotFound

from compose.cli.colors import AnsiMode
from compose.cli.colors import green
from compose.cli.colors import red
from compose.cli.signals import ShutdownException
from compose.const import PARALLEL_LIMIT
from compose.errors import CompletedUnsuccessfully
from compose.errors import HealthCheckFailed
from compose.errors import NoHealthCheckConfigured
from compose.errors import OperationFailedError


log = logging.getLogger(__name__)

STOP = object()


class GlobalLimit:
    """Simple class to hold a global semaphore limiter for a project. This class
    should be treated as a singleton that is instantiated when the project is.
    """

    global_limiter = Semaphore(PARALLEL_LIMIT)

    @classmethod
    def set_global_limit(cls, value):
        if value is None:
            value = PARALLEL_LIMIT
        cls.global_limiter = Semaphore(value)


def parallel_execute_watch(events, writer, errors, results, msg, get_name, fail_check):
    """ Watch events from a parallel execution, update status and fill errors and results.
        Returns exception to re-raise.
    """
    error_to_reraise = None
    for obj, result, exception in events:
        if exception is None:
            if fail_check is not None and fail_check(obj):
                writer.write(msg, get_name(obj), 'failed', red)
            else:
                writer.write(msg, get_name(obj), 'done', green)
            results.append(result)
        elif isinstance(exception, ImageNotFound):
            # This is to bubble up ImageNotFound exceptions to the client so we
            # can prompt the user if they want to rebuild.
            errors[get_name(obj)] = exception.explanation
            writer.write(msg, get_name(obj), 'error', red)
            error_to_reraise = exception
        elif isinstance(exception, APIError):
            errors[get_name(obj)] = exception.explanation
            writer.write(msg, get_name(obj), 'error', red)
        elif isinstance(exception, (OperationFailedError, HealthCheckFailed, NoHealthCheckConfigured,
                                    CompletedUnsuccessfully)):
            errors[get_name(obj)] = exception.msg
            writer.write(msg, get_name(obj), 'error', red)
        elif isinstance(exception, UpstreamError):
            writer.write(msg, get_name(obj), 'error', red)
        else:
            errors[get_name(obj)] = exception
            error_to_reraise = exception
    return error_to_reraise


def parallel_execute(objects, func, get_name, msg, get_deps=None, limit=None, fail_check=None):
    """Runs func on objects in parallel while ensuring that func is
    ran on object only after it is ran on all its dependencies.

    get_deps called on object must return a collection with its dependencies.
    get_name called on object must return its name.
    fail_check is an additional failure check for cases that should display as a failure
        in the CLI logs, but don't raise an exception (such as attempting to start 0 containers)
    """
    objects = list(objects)
    stream = sys.stderr

    writer = ParallelStreamWriter.get_or_assign_instance(ParallelStreamWriter(stream))

    for obj in objects:
        writer.add_object(msg, get_name(obj))
    for obj in objects:
        writer.write_initial(msg, get_name(obj))

    events = parallel_execute_iter(objects, func, get_deps, limit)

    errors = {}
    results = []
    error_to_reraise = parallel_execute_watch(
        events, writer, errors, results, msg, get_name, fail_check
    )

    for obj_name, error in errors.items():
        stream.write("\nERROR: for {}  {}\n".format(obj_name, error))

    if error_to_reraise:
        raise error_to_reraise

    return results, errors


def _no_deps(x):
    return []


class State:
    """
    Holds the state of a partially-complete parallel operation.

    state.started:   objects being processed
    state.finished:  objects which have been processed
    state.failed:    objects which either failed or whose dependencies failed
    """
    def __init__(self, objects):
        self.objects = objects

        self.started = set()
        self.finished = set()
        self.failed = set()

    def is_done(self):
        return len(self.finished) + len(self.failed) >= len(self.objects)

    def pending(self):
        return set(self.objects) - self.started - self.finished - self.failed


class NoLimit:
    def __enter__(self):
        pass

    def __exit__(self, *ex):
        pass


def parallel_execute_iter(objects, func, get_deps, limit):
    """
    Runs func on objects in parallel while ensuring that func is
    ran on object only after it is ran on all its dependencies.

    Returns an iterator of tuples which look like:

    # if func returned normally when run on object
    (object, result, None)

    # if func raised an exception when run on object
    (object, None, exception)

    # if func raised an exception when run on one of object's dependencies
    (object, None, UpstreamError())
    """
    if get_deps is None:
        get_deps = _no_deps

    if limit is None:
        limiter = NoLimit()
    else:
        limiter = Semaphore(limit)

    results = Queue()
    state = State(objects)

    while True:
        feed_queue(objects, func, get_deps, results, state, limiter)

        try:
            event = results.get(timeout=0.1)
        except Empty:
            continue
        # See https://github.com/docker/compose/issues/189
        except thread.error:
            raise ShutdownException()

        if event is STOP:
            break

        obj, _, exception = event
        if exception is None:
            log.debug('Finished processing: {}'.format(obj))
            state.finished.add(obj)
        else:
            log.debug('Failed: {}'.format(obj))
            state.failed.add(obj)

        yield event


def producer(obj, func, results, limiter):
    """
    The entry point for a producer thread which runs func on a single object.
    Places a tuple on the results queue once func has either returned or raised.
    """
    with limiter, GlobalLimit.global_limiter:
        try:
            result = func(obj)
            results.put((obj, result, None))
        except Exception as e:
            results.put((obj, None, e))


def feed_queue(objects, func, get_deps, results, state, limiter):
    """
    Starts producer threads for any objects which are ready to be processed
    (i.e. they have no dependencies which haven't been successfully processed).

    Shortcuts any objects whose dependencies have failed and places an
    (object, None, UpstreamError()) tuple on the results queue.
    """
    pending = state.pending()
    log.debug('Pending: {}'.format(pending))

    for obj in pending:
        deps = get_deps(obj)
        try:
            if any(dep[0] in state.failed for dep in deps):
                log.debug('{} has upstream errors - not processing'.format(obj))
                results.put((obj, None, UpstreamError()))
                state.failed.add(obj)
            elif all(
                dep not in objects or (
                    dep in state.finished and (not ready_check or ready_check(dep))
                ) for dep, ready_check in deps
            ):
                log.debug('Starting producer thread for {}'.format(obj))
                t = Thread(target=producer, args=(obj, func, results, limiter))
                t.daemon = True
                t.start()
                state.started.add(obj)
        except (HealthCheckFailed, NoHealthCheckConfigured) as e:
            log.debug(
                'Healthcheck for service(s) upstream of {} failed - '
                'not processing'.format(obj)
            )
            results.put((obj, None, e))
        except CompletedUnsuccessfully as e:
            log.debug(
                'Service(s) upstream of {} did not completed successfully - '
                'not processing'.format(obj)
            )
            results.put((obj, None, e))

    if state.is_done():
        results.put(STOP)


class UpstreamError(Exception):
    pass


class ParallelStreamWriter:
    """Write out messages for operations happening in parallel.

    Each operation has its own line, and ANSI code characters are used
    to jump to the correct line, and write over the line.
    """

    default_ansi_mode = AnsiMode.AUTO
    write_lock = Lock()

    instance = None
    instance_lock = Lock()

    @classmethod
    def get_instance(cls):
        return cls.instance

    @classmethod
    def get_or_assign_instance(cls, writer):
        cls.instance_lock.acquire()
        try:
            if cls.instance is None:
                cls.instance = writer
            return cls.instance
        finally:
            cls.instance_lock.release()

    @classmethod
    def set_default_ansi_mode(cls, ansi_mode):
        cls.default_ansi_mode = ansi_mode

    def __init__(self, stream, ansi_mode=None):
        if ansi_mode is None:
            ansi_mode = self.default_ansi_mode
        self.stream = stream
        self.use_ansi_codes = ansi_mode.use_ansi_codes(stream)
        self.lines = []
        self.width = 0

    def add_object(self, msg, obj_index):
        if msg is None:
            return
        self.lines.append(msg + obj_index)
        self.width = max(self.width, len(msg + ' ' + obj_index))

    def write_initial(self, msg, obj_index):
        if msg is None:
            return
        return self._write_noansi(msg, obj_index, '')

    def _write_ansi(self, msg, obj_index, status):
        self.write_lock.acquire()
        position = self.lines.index(msg + obj_index)
        diff = len(self.lines) - position
        # move up
        self.stream.write("%c[%dA" % (27, diff))
        # erase
        self.stream.write("%c[2K\r" % 27)
        self.stream.write("{:<{width}} ... {}\r".format(msg + ' ' + obj_index,
                          status, width=self.width))
        # move back down
        self.stream.write("%c[%dB" % (27, diff))
        self.stream.flush()
        self.write_lock.release()

    def _write_noansi(self, msg, obj_index, status):
        self.stream.write(
            "{:<{width}} ... {}\r\n".format(
                msg + ' ' + obj_index, status, width=self.width
            )
        )
        self.stream.flush()

    def write(self, msg, obj_index, status, color_func):
        if msg is None:
            return
        if self.use_ansi_codes:
            self._write_ansi(msg, obj_index, color_func(status))
        else:
            self._write_noansi(msg, obj_index, status)


def parallel_operation(containers, operation, options, message):
    parallel_execute(
        containers,
        operator.methodcaller(operation, **options),
        operator.attrgetter('name'),
        message,
    )


def parallel_remove(containers, options):
    stopped_containers = [c for c in containers if not c.is_running]
    parallel_operation(stopped_containers, 'remove', options, 'Removing')


def parallel_pause(containers, options):
    parallel_operation(containers, 'pause', options, 'Pausing')


def parallel_unpause(containers, options):
    parallel_operation(containers, 'unpause', options, 'Unpausing')


def parallel_kill(containers, options):
    parallel_operation(containers, 'kill', options, 'Killing')
