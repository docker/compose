import enum
import os

from ..const import IS_WINDOWS_PLATFORM

NAMES = [
    'grey',
    'red',
    'green',
    'yellow',
    'blue',
    'magenta',
    'cyan',
    'white'
]


@enum.unique
class AnsiMode(enum.Enum):
    """Enumeration for when to output ANSI colors."""
    NEVER = "never"
    ALWAYS = "always"
    AUTO = "auto"

    def use_ansi_codes(self, stream):
        if self is AnsiMode.ALWAYS:
            return True
        if self is AnsiMode.NEVER or os.environ.get('CLICOLOR') == '0':
            return False
        return stream.isatty()


def get_pairs():
    for i, name in enumerate(NAMES):
        yield (name, str(30 + i))
        yield ('intense_' + name, str(30 + i) + ';1')


def ansi(code):
    return '\033[{}m'.format(code)


def ansi_color(code, s):
    return '{}{}{}'.format(ansi(code), s, ansi(0))


def make_color_fn(code):
    return lambda s: ansi_color(code, s)


if IS_WINDOWS_PLATFORM:
    import colorama
    colorama.init(strip=False)
for (name, code) in get_pairs():
    globals()[name] = make_color_fn(code)


def rainbow():
    cs = ['cyan', 'yellow', 'green', 'magenta', 'blue',
          'intense_cyan', 'intense_yellow', 'intense_green',
          'intense_magenta', 'intense_blue']

    for c in cs:
        yield globals()[c]
