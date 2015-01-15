
from .. import unittest

from fig.includes import (
    FetchExternalConfigError,
    get_project_from_http,
    get_project_from_s3,
    normalize_url,
)


class IncludeHttpTest(unittest.TestCase):

    def test_get_project_from_http(self):
        # returns JSON, but yaml can parse that just fine
        url = normalize_url('http://httpbin.org/get')
        project = get_project_from_http(url, {})
        self.assertIn('url', project)

    def test_get_project_from_http_with_http_error(self):
        url = normalize_url('http://httpbin.org/status/404')
        with self.assertRaises(FetchExternalConfigError) as exc_context:
            get_project_from_http(url, {})
        self.assertEqual(
            'Failed to include http://httpbin.org/status/404: '
            '404 Client Error: NOT FOUND',
            str(exc_context.exception))

    def test_get_project_from_http_with_connection_error(self):
        url = normalize_url('http://hostdoesnotexist.bogus/')
        with self.assertRaises(FetchExternalConfigError) as exc_context:
            get_project_from_http(url, {'timeout': 2})
        self.assertIn('Name or service not known', str(exc_context.exception))
