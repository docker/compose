from .. import unittest
from compose import config
from compose.errors import ValidationError
from os import listdir, path


class TestInvalidFiles(unittest.TestCase):
    basepath = path.abspath(path.join(path.dirname(__file__), '..', 'fixtures', 'invalid_config'))

    def test_invalid_files(self):
        for f in [x for x in listdir(self.basepath) if x.endswith('.yml')]:
            self.assertRaises(ValidationError, config.load, path.join(self.basepath, f))


class TestServiceDicts(unittest.TestCase):
    def test_invalid_dicts(self):
        basepath = path.abspath(path.join(path.dirname(__file__), '..', 'fixtures', 'invalid_config'))
        dicts_file = path.join(basepath, 'invalid_service_dicts.yml')
        tests_dict = config.load_yaml(dicts_file)
        for k, v in tests_dict.items():
            try:
                config.from_dictionary({k: v}, basepath)
            except ValidationError, e:  # noqa
                pass
            else:
                self.fail('Didn`t raise ValidationError: {test_dict_name} = {test_dict}'.format(test_dict_name=k, test_dict=v))

    def test_valid_dicts(self):
        basepath = path.abspath(path.join(path.dirname(__file__), '..', 'fixtures', 'valid_config'))
        dicts_file = path.join(basepath, 'valid_service_dicts.yml')
        tests_dict = config.load_yaml(dicts_file)
        for k in tests_dict.keys():
            try:
                config.from_dictionary({k: tests_dict[k]}, basepath)
            except ValidationError, e:
                self.fail(e.msg)
