import codecs
import os
import shutil
import tempfile

from ddt import data
from ddt import ddt
from ddt import unpack

from compose.config.environment import env_vars_from_file
from compose.config.environment import Environment
from tests import unittest


@ddt
class EnvironmentTest(unittest.TestCase):
    @classmethod
    def test_get_simple(self):
        env = Environment({
            'FOO': 'bar',
            'BAR': '1',
            'BAZ': ''
        })

        assert env.get('FOO') == 'bar'
        assert env.get('BAR') == '1'
        assert env.get('BAZ') == ''

    @classmethod
    def test_get_undefined(self):
        env = Environment({
            'FOO': 'bar'
        })
        assert env.get('FOOBAR') is None

    @classmethod
    def test_get_boolean(self):
        env = Environment({
            'FOO': '',
            'BAR': '0',
            'BAZ': 'FALSE',
            'FOOBAR': 'true',
        })

        assert env.get_boolean('FOO') is False
        assert env.get_boolean('BAR') is False
        assert env.get_boolean('BAZ') is False
        assert env.get_boolean('FOOBAR') is True
        assert env.get_boolean('UNDEFINED') is False

    @data(
        ('unicode exclude test', '\ufeffPARK_BOM=박봄\n', {'PARK_BOM': '박봄'}),
        ('export prefixed test', 'export PREFIXED_VARS=yes\n', {"PREFIXED_VARS": "yes"}),
        ('quoted vars test', "QUOTED_VARS='yes'\n", {"QUOTED_VARS": "yes"}),
        ('double quoted vars test', 'DOUBLE_QUOTED_VARS="yes"\n', {"DOUBLE_QUOTED_VARS": "yes"}),
        ('extra spaces test', 'SPACES_VARS = "yes"\n', {"SPACES_VARS": "yes"}),
    )
    @unpack
    def test_env_vars(self, test_name, content, expected):
        tmpdir = tempfile.mkdtemp('env_file')
        self.addCleanup(shutil.rmtree, tmpdir)
        file_abs_path = str(os.path.join(tmpdir, ".env"))
        with codecs.open(file_abs_path, 'w', encoding='utf-8') as f:
            f.write(content)
        assert env_vars_from_file(file_abs_path) == expected, '"{}" Failed'.format(test_name)
