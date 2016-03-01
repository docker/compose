from __future__ import absolute_import
from __future__ import unicode_literals

import pytest
import six
from six.moves.queue import Queue

from compose.cli.log_printer import consume_queue
from compose.cli.log_printer import LogPrinter
from compose.cli.log_printer import STOP
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
    def reader(*args, **kwargs):
        yield b"hello\nworld"
    return build_mock_container(reader)


@pytest.mark.skipif(True, reason="wip")
class TestLogPrinter(object):

    def test_single_container(self, output_stream, mock_container):
        LogPrinter([mock_container], output=output_stream, log_args={'follow': True}).run()

        output = output_stream.getvalue()
        assert 'hello' in output
        assert 'world' in output
        # Call count is 2 lines + "container exited line"
        assert output_stream.flush.call_count == 3

    def test_single_container_without_stream(self, output_stream, mock_container):
        LogPrinter([mock_container], output=output_stream).run()

        output = output_stream.getvalue()
        assert 'hello' in output
        assert 'world' in output
        # Call count is 2 lines
        assert output_stream.flush.call_count == 2

    def test_monochrome(self, output_stream, mock_container):
        LogPrinter([mock_container], output=output_stream, monochrome=True).run()
        assert '\033[' not in output_stream.getvalue()

    def test_polychrome(self, output_stream, mock_container):
        LogPrinter([mock_container], output=output_stream).run()
        assert '\033[' in output_stream.getvalue()

    def test_unicode(self, output_stream):
        glyph = u'\u2022'

        def reader(*args, **kwargs):
            yield glyph.encode('utf-8') + b'\n'

        container = build_mock_container(reader)
        LogPrinter([container], output=output_stream).run()
        output = output_stream.getvalue()
        if six.PY2:
            output = output.decode('utf-8')

        assert glyph in output

    def test_wait_on_exit(self):
        exit_status = 3
        mock_container = mock.Mock(
            spec=Container,
            name='cname',
            wait=mock.Mock(return_value=exit_status))

        expected = '{} exited with code {}\n'.format(mock_container.name, exit_status)
        assert expected == wait_on_exit(mock_container)

    def test_generator_with_no_logs(self, mock_container, output_stream):
        mock_container.has_api_logs = False
        mock_container.log_driver = 'none'
        LogPrinter([mock_container], output=output_stream).run()

        output = output_stream.getvalue()
        assert "WARNING: no logs are available with the 'none' log driver\n" in output
        assert "exited with code" not in output


class TestConsumeQueue(object):

    def test_item_is_an_exception(self):

        class Problem(Exception):
            pass

        queue = Queue()
        error = Problem('oops')
        for item in ('a', None), ('b', None), (None, error):
            queue.put(item)

        generator = consume_queue(queue, False)
        assert generator.next() == 'a'
        assert generator.next() == 'b'
        with pytest.raises(Problem):
            generator.next()

    def test_item_is_stop_without_cascade_stop(self):
        queue = Queue()
        for item in (STOP, None), ('a', None), ('b', None):
            queue.put(item)

        generator = consume_queue(queue, False)
        assert generator.next() == 'a'
        assert generator.next() == 'b'

    def test_item_is_stop_with_cascade_stop(self):
        queue = Queue()
        for item in (STOP, None), ('a', None), ('b', None):
            queue.put(item)

        assert list(consume_queue(queue, True)) == []
