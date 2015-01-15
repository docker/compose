import contextlib
import os.path
import shutil
import tempfile

import boto.exception
import boto.s3.connection
import mock
from .. import unittest

from fig.includes import (
    ExternalProjectCache,
    FetchExternalConfigError,
    LocalConfigCache,
    fetch_external_config,
    get_project_from_file,
    get_project_from_s3,
    normalize_url,
    url_to_filename,
)
from fig.project import Project
from fig.service import ConfigError


class NormalizeUrlTest(unittest.TestCase):

    def test_normalize_url_with_scheme(self):
        url = normalize_url('HTTPS://example.com')
        self.assertEqual(url.scheme, 'https')

    def test_normalize_url_without_scheme(self):
        url = normalize_url('./path/to/somewhere')
        self.assertEqual(url.scheme, 'file')


class GetProjectFromS3Test(unittest.TestCase):

    @mock.patch('fig.includes.get_boto_conn', autospec=True)
    def test_get_project_from_s3(self, mock_get_conn):
        mock_bucket = mock_get_conn.return_value.get_bucket.return_value
        mock_key = mock_bucket.get_key.return_value
        mock_key.get_contents_as_string.return_value = 'foo:\n  build: .'
        url = normalize_url('s3://bucket/path/to/key/fig.yml')

        project = get_project_from_s3(url)
        self.assertEqual(project, {'foo': {'build': '.'}})

        mock_get_conn.assert_called_once_with()
        mock_get_conn.return_value.get_bucket.assert_called_once_with('bucket')
        mock_bucket.get_key.assert_called_once_with('/path/to/key/fig.yml')


    @mock.patch('fig.includes.get_boto_conn', autospec=True)
    def test_get_project_from_s3_not_found(self, mock_get_conn):
        mock_bucket = mock_get_conn.return_value.get_bucket.return_value
        mock_bucket.get_key.return_value = None
        url = normalize_url('s3://bucket/path/to/key/fig.yml')

        with self.assertRaises(FetchExternalConfigError) as exc_context:
            get_project_from_s3(url)
        self.assertEqual(
            "Failed to include %s: Not Found" % url.geturl(),
            str(exc_context.exception))

    @mock.patch('fig.includes.get_boto_conn', autospec=True)
    def test_get_project_from_s3_bucket_error(self, mock_get_conn):
        mock_get_bucket = mock_get_conn.return_value.get_bucket
        mock_get_bucket.side_effect = boto.exception.S3ResponseError(
            404, "Bucket Not Found")

        url = normalize_url('s3://bucket/path/to/key/fig.yml')
        with self.assertRaises(FetchExternalConfigError) as exc_context:
            get_project_from_s3(url)
        self.assertEqual(
            "Failed to include %s: S3ResponseError: 404 Bucket Not Found\n" %
                url.geturl(), str(exc_context.exception))


class FetchExternalConfigTest(unittest.TestCase):

    def test_unsupported_scheme(self):
        with self.assertRaises(ConfigError) as exc:
            fetch_external_config(normalize_url("bogus://something"), None)
        self.assertIn("bogus", str(exc.exception))

    def test_fetch_from_file(self):
        url = "./tests/fixtures/external-includes-figfile/fig.yml"
        config = fetch_external_config(normalize_url(url), None)
        self.assertEqual(
            set(config.keys()),
            set(['db', 'webapp', 'project-config']))


class GetProjectFromFileWithNormalizeUrlTest(unittest.TestCase):

    def setUp(self):
        self.expected = set(['db', 'webapp', 'project-config'])
        self.path = "tests/fixtures/external-includes-figfile/fig.yml"

    def test_fetch_from_file_relative_no_context(self):
        config = get_project_from_file(normalize_url(self.path))
        self.assertEqual(set(config.keys()), self.expected)

    def test_fetch_from_file_relative_with_context(self):
        url = './' + self.path
        config = get_project_from_file(normalize_url(url))
        self.assertEqual(set(config.keys()), self.expected)

    def test_fetch_from_file_absolute_path(self):
        url = os.path.abspath(self.path)
        config = get_project_from_file(normalize_url(url))
        self.assertEqual(set(config.keys()), self.expected)

    def test_fetch_from_file_relative_with_scheme(self):
        url = 'file://./' + self.path
        config = get_project_from_file(normalize_url(url))
        self.assertEqual(set(config.keys()), self.expected)

    def test_fetch_from_file_absolute_with_scheme(self):
        url = 'file://' + os.path.abspath(self.path)
        config = get_project_from_file(normalize_url(url))
        self.assertEqual(set(config.keys()), self.expected)


class LocalConfigCacheTest(unittest.TestCase):

    url = "http://example.com/path/to/file.yml"

    def setUp(self):
        self.path = '/base/path'
        self.ttl = 20

    def test_url_to_filename(self):
        path = '/base'
        filename = url_to_filename(path, normalize_url(self.url))
        expected = '/base/http%3A%2F%2Fexample.com%2Fpath%2Fto%2Ffile.yml'
        self.assertEqual(filename, expected)

    @mock.patch('fig.includes.time.time', autospec=True)
    def test_is_fresh_false(self, mock_time):
        cache = LocalConfigCache(self.path, self.ttl)
        mock_time.return_value = 1000
        self.assertFalse(cache.is_fresh(900))

    @mock.patch('fig.includes.time.time', autospec=True)
    def test_is_fresh_true(self, mock_time):
        cache = LocalConfigCache(self.path, self.ttl)
        mock_time.return_value = 1000
        self.assertTrue(cache.is_fresh(990))

    @mock.patch('fig.includes.os.path.isdir', autospec=True)
    def test_from_config(self, mock_isdir):
        mock_isdir.return_value = True
        cache = LocalConfigCache.from_config({
            'ttl': '6 min',
            'path': '~/.cache-path'
        })
        self.assertEqual(cache.ttl, 360)
        self.assertEqual(cache.path, os.path.expandvars('$HOME/.cache-path'))

    def test_get_and_set(self):
        url = normalize_url(self.url)
        config = {'foo': {'image': 'busybox'}}

        with temp_dir() as path:
            cache = LocalConfigCache(path, self.ttl)

            self.assertNotIn(url, cache)
            cache[url] = config
            self.assertIn(url, cache)
            self.assertEqual(config, cache[url])


@contextlib.contextmanager
def temp_dir():
    path = tempfile.mkdtemp()
    try:
        yield path
    finally:
        shutil.rmtree(path)


class ExternalProjectCacheTest(unittest.TestCase):

    project_b = {
        'url': 'http://example.com/project_b/fig.yml'
    }

    def setUp(self):
        self.client = mock.Mock()
        self.mock_factory = mock.create_autospec(Project.from_config)
        self.externals = ExternalProjectCache({}, self.client, self.mock_factory)

    def test_get_project_from_include_invalid_config(self):
        with self.assertRaises(ConfigError) as exc:
            self.externals.get_project_from_include('something', {})
        self.assertIn(
            "Project include 'something' requires a url",
            str(exc.exception))

    @mock.patch('fig.includes.fetch_external_config', autospec=True)
    def test_get_external_projects_no_cache(self, mock_fetch):
        name = 'project_b'
        project = self.externals.get_project_from_include(name, self.project_b)

        url = normalize_url(self.project_b['url'])
        mock_fetch.assert_called_once_with(url, self.project_b)

        self.mock_factory.assert_called_once_with(
            name,
            {},
            self.client,
            project_cache=self.externals)

        self.assertEqual(project, self.mock_factory.return_value)
        self.assertIn(url, self.externals.config_cache)
        self.assertIn(url, self.externals.project_cache)

    @mock.patch('fig.includes.fetch_external_config', autospec=True)
    def test_get_external_projects_from_cache(self, mock_fetch):
        mock_project = mock.create_autospec(Project)
        url = normalize_url(self.project_b['url'])
        self.externals.project_cache[url] = mock_project

        name = 'project_b'
        project = self.externals.get_project_from_include(name, self.project_b)

        self.assertEqual(mock_fetch.called, False)
        self.assertEqual(self.mock_factory.called, False)
        self.assertEqual(project, mock_project)

    def test_build_project_with_build_directive(self):
        config = {
            'foo': {'build': '.'},
            'other': {'image': 'busybox'},
        }
        self.externals.factory = Project.from_config
        with mock.patch('fig.includes.log', autospec=True) as mock_log:
            project = self.externals.build_project('project', config)
            self.assertEqual(len(project.services), 1)
            self.assertEqual(project.services[0].full_name, 'project_other')

            mock_log.info.assert_called_once_with(
                "Service project_foo is external and uses build, skipping")

    def test_get_config_from_config_cache(self):
        url = normalize_url(self.project_b['url'])
        mock_config = mock.Mock()
        self.externals.config_cache = {url: mock_config}

        config = self.externals.get_config(url, self.project_b)
        self.assertEqual(mock_config, config)
