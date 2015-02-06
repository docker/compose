"""Include external projects, allowing services to link to a service
defined in an external project.
"""
import logging
import os
import time

from pytimeparse import timeparse
import requests
import requests.exceptions
import six
from six.moves.urllib.parse import urlparse, quote
import yaml

from fig.service import ConfigError


log = logging.getLogger(__name__)


class FetchExternalConfigError(Exception):
    pass


def normalize_url(url):
    url = urlparse(url)
    return url if url.scheme else url._replace(scheme='file')


def read_config(content):
    return yaml.safe_load(content)


def get_project_from_file(url):
    # Handle urls in the form file://./some/relative/path
    path = url.netloc + url.path if url.netloc.startswith('.') else url.path
    with open(path, 'r') as fh:
        return read_config(fh.read())


def get_project_from_http(url, config):
    try:
        response = requests.get(
            url.geturl(),
            timeout=config.get('timeout', 20),
            verify=config.get('verify_ssl_cert', True),
            cert=config.get('ssl_cert', None),
            proxies=config.get('proxies', None))
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        raise FetchExternalConfigError("Failed to include %s: %s" % (
            url.geturl(), e))
    return read_config(response.text)


# Return the connection from a function, so it can be mocked in tests
def get_boto_conn():
    # Local import so that boto is only a dependency if it's used
    import boto.s3.connection
    return boto.s3.connection.S3Connection()


def get_project_from_s3(url):
    import boto.exception
    try:
        conn = get_boto_conn()
        bucket = conn.get_bucket(url.netloc)
    except (boto.exception.BotoServerError, boto.exception.BotoClientError) as e:
        raise FetchExternalConfigError(
            "Failed to include %s: %s" % (url.geturl(), e))

    key = bucket.get_key(url.path)
    if not key:
        raise FetchExternalConfigError(
            "Failed to include %s: Not Found" % url.geturl())

    return read_config(key.get_contents_as_string())


def fetch_external_config(url, include_config):
    log.info("Fetching config from %s" % url.geturl())

    if url.scheme in ('http', 'https'):
        return get_project_from_http(url, include_config)

    if url.scheme == 'file':
        return get_project_from_file(url)

    if url.scheme == 's3':
        return get_project_from_s3(url)

    raise ConfigError("Unsupported url scheme \"%s\" for %s." % (
        url.scheme,
        url))


class LocalConfigCache(object):

    def __init__(self, path, ttl):
        self.path = path
        self.ttl = ttl

    @classmethod
    def from_config(cls, cache_config):
        if not cache_config.get('enable', True):
            return {}

        path = os.path.expanduser(cache_config.get('path', '~/.fig-cache'))
        ttl = timeparse.timeparse(cache_config.get('ttl', '5 min'))

        if not os.path.isdir(path):
            try:
                os.makedirs(path)
            except OSError:
                # Handle the race condition where some other process creates
                # this directory after the isdir check
                if not os.path.isdir(path):
                    raise

        if ttl is None:
            raise ConfigError("Cache ttl \'%s\' could not be parsed" %
                              cache_config.get('ttl'))

        return cls(path, ttl)

    def is_fresh(self, mtime):
        return mtime + self.ttl > time.time()

    def __contains__(self, url):
        path = url_to_filename(self.path, url)
        return os.path.exists(path) and self.is_fresh(os.path.getmtime(path))

    def __getitem__(self, url):
        if url not in self:
            raise KeyError(url)
        with open(url_to_filename(self.path, url), 'r') as fh:
            return read_config(fh.read())

    def __setitem__(self, url, contents):
        with open(url_to_filename(self.path, url), 'w') as fh:
            return fh.write(yaml.dump(contents))


def url_to_filename(path, url):
    return os.path.join(path, quote(url.geturl(), safe=''))


class ExternalProjectCache(object):
    """Cache each Project by the url used to retreive the projects fig.yml.
    If multiple projects include the same url, re-use the same instance of the
    project.
    """

    def __init__(self, cache, client, factory):
        self.config_cache = cache
        self.project_cache = {}
        self.client = client
        self.factory = factory

    def get_project_from_include(self, name, include):
        if 'url' not in include:
            raise ConfigError("Project include '%s' requires a url" % name)
        url = normalize_url(include['url'])

        if url not in self.project_cache:
            config = self.get_config(url, include)
            self.project_cache[url] = self.build_project(name, config)

        return self.project_cache[url]

    def get_config(self, url, include):
        if url in self.config_cache:
            return self.config_cache[url]

        self.config_cache[url] = config = fetch_external_config(url, include)
        return config

    def build_project(self, name, config):
        def is_build_service(service_name, service):
            if 'build' not in service:
                return False
            log.info("Service %s_%s is external and uses build, skipping" % (
                name,
                service_name))
            return True

        config = dict(
            (name, service) for name, service in six.iteritems(config)
            if not is_build_service(name, service))
        return self.factory(name, config, self.client, project_cache=self)
