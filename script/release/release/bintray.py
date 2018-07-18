from __future__ import absolute_import
from __future__ import unicode_literals

import json

import requests

from .const import NAME


class BintrayAPI(requests.Session):
    def __init__(self, api_key, user, *args, **kwargs):
        super(BintrayAPI, self).__init__(*args, **kwargs)
        self.auth = (user, api_key)
        self.base_url = 'https://api.bintray.com/'

    def create_repository(self, subject, repo_name, repo_type='generic'):
        url = '{base}repos/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name,
        )
        data = {
            'name': repo_name,
            'type': repo_type,
            'private': False,
            'desc': 'Automated release for {}: {}'.format(NAME, repo_name),
            'labels': ['docker-compose', 'docker', 'release-bot'],
        }
        return self.post_json(url, data)

    def repository_exists(self, subject, repo_name):
        url = '{base}/repos/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name,
        )
        result = self.get(url)
        if result.status_code == 404:
            return False
        result.raise_for_status()
        return True

    def delete_repository(self, subject, repo_name):
        url = '{base}repos/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name,
        )
        return self.delete(url)

    def post_json(self, url, data, **kwargs):
        if 'headers' not in kwargs:
            kwargs['headers'] = {}
        kwargs['headers']['Content-Type'] = 'application/json'
        return self.post(url, data=json.dumps(data), **kwargs)
