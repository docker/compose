from __future__ import absolute_import
from __future__ import unicode_literals

import pytest
import six
from six.moves.queue import Queue

from compose.cli.log_printer import build_log_generator
from compose.cli.log_printer import build_log_presenters
from compose.cli.log_printer import build_no_log_generator
from compose.cli.log_printer import consume_queue
from compose.cli.log_printer import QueueItem
from compose.cli.log_printer import wait_on_exit
from compose.container import Container
from tests import mock


def build_mock_container(reader):
    return mock.Mock(
        spec=Container,
        name='myapp_web_1',
        name_without_project='web_1',
        has_api_logs=True,
        log_stream=None,
        logs=reader,
        wait=mock.Mock(return_value=0),
    )


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
        presenter = presenters.next()
        actual = presenter.present(mock_container, "this line")
        assert actual == "web_1  | this line"

    def test_polychrome(self, mock_container):
        presenters = build_log_presenters(['foo', 'bar'], False)
        presenter = presenters.next()
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
        assert generator.next() == "hello\n"
        assert generator.next() == "world"
        mock_container.logs.assert_called_once_with(
            stdout=True,
            stderr=True,
            stream=True,
            **log_args)

    def test_with_log_stream(self, mock_container):
        mock_container.log_stream = iter([b"hello\nworld"])
        log_args = {'follow': True}

        generator = build_log_generator(mock_container, log_args)
        assert generator.next() == "hello\n"
        assert generator.next() == "world"

    def test_unicode(self, output_stream):
        glyph = u'\u2022\n'
        mock_container.log_stream = iter([glyph.encode('utf-8')])

        generator = build_log_generator(mock_container, {})
        assert generator.next() == glyph


class TestConsumeQueue(object):

    def test_item_is_an_exception(self):

        class Problem(Exception):
            pass

        queue = Queue()
        error = Problem('oops')
        for item in QueueItem.new('a'), QueueItem.new('b'), QueueItem.exception(error):
            queue.put(item)

        generator = consume_queue(queue, False)
        assert generator.next() == 'a'
        assert generator.next() == 'b'
        with pytest.raises(Problem):
            generator.next()

    def test_item_is_stop_without_cascade_stop(self):
        queue = Queue()
        for item in QueueItem.stop(), QueueItem.new('a'), QueueItem.new('b'):
            queue.put(item)

        generator = consume_queue(queue, False)
        assert generator.next() == 'a'
        assert generator.next() == 'b'

    def test_item_is_stop_with_cascade_stop(self):
        queue = Queue()
        for item in QueueItem.stop(), QueueItem.new('a'), QueueItem.new('b'):
            queue.put(item)

        assert list(consume_queue(queue, True)) == []

    def test_item_is_none_when_timeout_is_hit(self):
        queue = Queue()
        generator = consume_queue(queue, False)
        assert generator.next() is None
