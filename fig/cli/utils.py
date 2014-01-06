from __future__ import unicode_literals
from __future__ import absolute_import
from __future__ import division
import datetime
import os
import socket
from .errors import UserError


def cached_property(f):
    """
    returns a cached property that is calculated by function f
    http://code.activestate.com/recipes/576563-cached-property/
    """
    def get(self):
        try:
            return self._property_cache[f]
        except AttributeError:
            self._property_cache = {}
            x = self._property_cache[f] = f(self)
            return x
        except KeyError:
            x = self._property_cache[f] = f(self)
            return x

    return property(get)


def yesno(prompt, default=None):
    """
    Prompt the user for a yes or no.

    Can optionally specify a default value, which will only be
    used if they enter a blank line.

    Unrecognised input (anything other than "y", "n", "yes",
    "no" or "") will return None.
    """
    answer = raw_input(prompt).strip().lower()

    if answer == "y" or answer == "yes":
        return True
    elif answer == "n" or answer == "no":
        return False
    elif answer == "":
        return default
    else:
        return None


# http://stackoverflow.com/a/5164027
def prettydate(d):
    diff = datetime.datetime.utcnow() - d
    s = diff.seconds
    if diff.days > 7 or diff.days < 0:
        return d.strftime('%d %b %y')
    elif diff.days == 1:
        return '1 day ago'
    elif diff.days > 1:
        return '{0} days ago'.format(diff.days)
    elif s <= 1:
        return 'just now'
    elif s < 60:
        return '{0} seconds ago'.format(s)
    elif s < 120:
        return '1 minute ago'
    elif s < 3600:
        return '{0} minutes ago'.format(s/60)
    elif s < 7200:
        return '1 hour ago'
    else:
        return '{0} hours ago'.format(s/3600)


def mkdir(path, permissions=0o700):
    if not os.path.exists(path):
        os.mkdir(path)

    os.chmod(path, permissions)

    return path


def docker_url():
    if os.environ.get('DOCKER_URL'):
        return os.environ['DOCKER_URL']

    socket_path = '/var/run/docker.sock'
    tcp_hosts = [
        ('localdocker', 4243),
        ('127.0.0.1', 4243),
    ]
    tcp_host = '127.0.0.1'
    tcp_port = 4243

    if os.path.exists(socket_path):
        return 'unix://%s' % socket_path

    for host, port in tcp_hosts:
        try:
            s = socket.create_connection((host, port), timeout=1)
            s.close()
            return 'http://%s:%s' % (host, port)
        except:
            pass

    raise UserError("""
Couldn't find Docker daemon - tried:

unix://%s
%s

If it's running elsewhere, specify a url with DOCKER_URL.
    """ % (socket_path, '\n'.join('tcp://%s:%s' % h for h in tcp_hosts)))
