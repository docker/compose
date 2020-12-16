import os

import pytest

from compose.cli.colors import AnsiMode
from tests import mock


@pytest.fixture
def tty_stream():
    stream = mock.Mock()
    stream.isatty.return_value = True
    return stream


@pytest.fixture
def non_tty_stream():
    stream = mock.Mock()
    stream.isatty.return_value = False
    return stream


class TestAnsiModeTestCase:

    @mock.patch.dict(os.environ)
    def test_ansi_mode_never(self, tty_stream, non_tty_stream):
        if "CLICOLOR" in os.environ:
            del os.environ["CLICOLOR"]
        assert not AnsiMode.NEVER.use_ansi_codes(tty_stream)
        assert not AnsiMode.NEVER.use_ansi_codes(non_tty_stream)

        os.environ["CLICOLOR"] = "0"
        assert not AnsiMode.NEVER.use_ansi_codes(tty_stream)
        assert not AnsiMode.NEVER.use_ansi_codes(non_tty_stream)

    @mock.patch.dict(os.environ)
    def test_ansi_mode_always(self, tty_stream, non_tty_stream):
        if "CLICOLOR" in os.environ:
            del os.environ["CLICOLOR"]
        assert AnsiMode.ALWAYS.use_ansi_codes(tty_stream)
        assert AnsiMode.ALWAYS.use_ansi_codes(non_tty_stream)

        os.environ["CLICOLOR"] = "0"
        assert AnsiMode.ALWAYS.use_ansi_codes(tty_stream)
        assert AnsiMode.ALWAYS.use_ansi_codes(non_tty_stream)

    @mock.patch.dict(os.environ)
    def test_ansi_mode_auto(self, tty_stream, non_tty_stream):
        if "CLICOLOR" in os.environ:
            del os.environ["CLICOLOR"]
        assert AnsiMode.AUTO.use_ansi_codes(tty_stream)
        assert not AnsiMode.AUTO.use_ansi_codes(non_tty_stream)

        os.environ["CLICOLOR"] = "0"
        assert not AnsiMode.AUTO.use_ansi_codes(tty_stream)
        assert not AnsiMode.AUTO.use_ansi_codes(non_tty_stream)
