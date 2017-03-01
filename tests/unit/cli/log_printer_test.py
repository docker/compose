from __future__ import absolute_import
from __future__ import unicode_literals

import itertools

import pytest
import requests
import six
from docker.errors import APIError
from six.moves.queue import Queue

from compose.cli.log_printer import build_log_generator
from compose.cli.log_printer import build_log_presenters
from compose.cli.log_printer import build_no_log_generator
from compose.cli.log_printer import consume_queue
from compose.cli.log_printer import QueueItem
from compose.cli.log_printer import wait_on_exit
from compose.cli.log_printer import watch_events
from compose.container import Container
from tests import mock


@pytest.fixture
def output_stream():
    output = six.StringIO()
    output.flush = mock.Mock()
    return output


@pytest.fixture
def mock_container():
    return mock.Mock(spec=Container, name_without_project='web_1')


class TestLogPresenter(object):

    def test_monochrome(self, mock_container):
        presenters = build_log_presenters(['foo', 'bar'], True)
        presenter = next(presenters)
        actual = presenter.present(mock_container, "this line")
        assert actual == "web_1  | this line"

    def test_polychrome(self, mock_container):
        presenters = build_log_presenters(['foo', 'bar'], False)
        presenter = next(presenters)
        actual = presenter.present(mock_container, "this line")
        assert '\033[' in actual


def test_wait_on_exit():
    exit_status = 3
    mock_container = mock.Mock(
        spec=Container,
        name='cname',
        wait=mock.Mock(return_value=exit_status))

    expected = '{} exited with code {}\n'.format(mock_container.name, exit_status)
    assert expected == wait_on_exit(mock_container)


def test_wait_on_exit_raises():
    status_code = 500

    def mock_wait():
        resp = requests.Response()
        resp.status_code = status_code
        raise APIError('Bad server', resp)

    mock_container = mock.Mock(
        spec=Container,
        name='cname',
        wait=mock_wait
    )

    expected = 'Unexpected API error for {} (HTTP code {})\n'.format(
        mock_container.name, status_code,
    )
    assert expected in wait_on_exit(mock_container)


def test_build_no_log_generator(mock_container):
    mock_container.has_api_logs = False
    mock_container.log_driver = 'none'
    output, = build_no_log_generator(mock_container, None)
    assert "WARNING: no logs are available with the 'none' log driver\n" in output
    assert "exited with code" not in output


class TestBuildLogGenerator(object):

    def test_no_log_stream(self, mock_container):
        mock_container.log_stream = None
        mock_container.logs.return_value = iter([b"hello\nworld"])
        log_args = {'follow': True}

        generator = build_log_generator(mock_container, log_args)
        assert next(generator) == "hello\n"
        assert next(generator) == "world"
        mock_container.logs.assert_called_once_with(
            stdout=True,
            stderr=True,
            stream=True,
            **log_args)

    def test_with_log_stream(self, mock_container):
        mock_container.log_stream = iter([b"hello\nworld"])
        log_args = {'follow': True}

        generator = build_log_generator(mock_container, log_args)
        assert next(generator) == "hello\n"
        assert next(generator) == "world"

    def test_unicode(self, output_stream):
        glyph = u'\u2022\n'
        mock_container.log_stream = iter([glyph.encode('utf-8')])

        generator = build_log_generator(mock_container, {})
        assert next(generator) == glyph


@pytest.fixture
def thread_map():
    return {'cid': mock.Mock()}


@pytest.fixture
def mock_presenters():
    return itertools.cycle([mock.Mock()])


class TestWatchEvents(object):

    def test_stop_event(self, thread_map, mock_presenters):
        event_stream = [{'action': 'stop', 'id': 'cid'}]
        watch_events(thread_map, event_stream, mock_presenters, ())
        assert not thread_map

    def test_start_event(self, thread_map, mock_presenters):
        container_id = 'abcd'
        event = {'action': 'start', 'id': container_id, 'container': mock.Mock()}
        event_stream = [event]
        thread_args = 'foo', 'bar'

        with mock.patch(
            'compose.cli.log_printer.build_thread',
            autospec=True
        ) as mock_build_thread:
            watch_events(thread_map, event_stream, mock_presenters, thread_args)
            mock_build_thread.assert_called_once_with(
                event['container'],
                next(mock_presenters),
                *thread_args)
        assert container_id in thread_map

    def test_other_event(self, thread_map, mock_presenters):
        container_id = 'abcd'
        event_stream = [{'action': 'create', 'id': container_id}]
        watch_events(thread_map, event_stream, mock_presenters, ())
        assert container_id not in thread_map


class TestConsumeQueue(object):

    def test_item_is_an_exception(self):

        class Problem(Exception):
            pass

        queue = Queue()
        error = Problem('oops')
        for item in QueueItem.new('a'), QueueItem.new('b'), QueueItem.exception(error):
            queue.put(item)

        generator = consume_queue(queue, False)
        assert next(generator) == 'a'
        assert next(generator) == 'b'
        with pytest.raises(Problem):
            next(generator)

    def test_item_is_stop_without_cascade_stop(self):
        queue = Queue()
        for item in QueueItem.stop(), QueueItem.new('a'), QueueItem.new('b'):
            queue.put(item)

        generator = consume_queue(queue, False)
        assert next(generator) == 'a'
        assert next(generator) == 'b'

    def test_item_is_stop_with_cascade_stop(self):
        """Return the name of the container that caused the cascade_stop"""
        queue = Queue()
        for item in QueueItem.stop('foobar-1'), QueueItem.new('a'), QueueItem.new('b'):
            queue.put(item)

        generator = consume_queue(queue, True)
        assert next(generator) is 'foobar-1'

    def test_item_is_none_when_timeout_is_hit(self):
        queue = Queue()
        generator = consume_queue(queue, False)
        assert next(generator) is None
