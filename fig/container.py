from __future__ import unicode_literals
from __future__ import absolute_import

def flatten(d):
    """
        Transform data like this:
            [('k0', 'v0'), [('k1', 'v1'),('k2', 'v2')], [[('k3', 'v3')]]]
        into dictionary:
            {
                'k0': 'v0',
                'k1': 'v1',
                'k2': 'v2',
                'k3': 'v3',
            }
    """
    g = {}
    def _inner(_d):
        if isinstance(_d, tuple):
            g[_d[0]] = _d[1]
        elif isinstance(_d, list):
            map(_inner, _d)
    _inner(d)
    return g

def prepare_d(d, prefix=u''):
    """
        Return list from nested data structures like this:
        {
            'Param1':{
                'ListParam':['a','b', 'c'],
                'DictParam':{
                    'ListParam':['a', 'b', 'c'],
                    'NumberParam': 100,
                    'BoolParam':False,
                    'EmptyDictParam':{},
                    'EmptyListParam':[]
                }
            }
        }
        into this
        [
            [
                [
                    (u'.Param1.ListParam.0', u'a'),
                    (u'.Param1.ListParam.1', u'b'),
                    (u'.Param1.ListParam.2', u'c')
                ],
                [
                    (u'.Param1.DictParam.EmptyDictParam', None),
                    (u'.Param1.DictParam.NumberParam', u'100'),
                    [
                        (u'.Param1.DictParam.ListParam.0', u'a'),
                        (u'.Param1.DictParam.ListParam.1', u'b'),
                        (u'.Param1.DictParam.ListParam.2', u'c')
                    ],
                    (u'.Param1.DictParam.EmptyListParam', None),
                    (u'.Param1.DictParam.BoolParam', u'False')
                ]
            ]
        ]
    """
    if isinstance(d, dict):
        if len(d):
            return map(lambda (key, value): prepare_d(value, u'%s.%s' % (prefix, key)), d.items())
        return (prefix, None)
    elif isinstance(d, list):
        if len(d) > 0:
            return map(lambda (key, value): prepare_d(value, u'%s.%s' % (prefix, key)), enumerate(d))
        return (prefix, None)
    return (prefix, unicode(d))


class Container(object):
    """
    Represents a Docker container, constructed from the output of
    GET /containers/:id:/json.
    """
    def __init__(self, client, dictionary, has_been_inspected=False):
        self.client = client
        self.dictionary = dictionary
        self.has_been_inspected = has_been_inspected

    @classmethod
    def from_ps(cls, client, dictionary, **kwargs):
        """
        Construct a container object from the output of GET /containers/json.
        """
        new_dictionary = {
            'ID': dictionary['Id'],
            'Image': dictionary['Image'],
        }
        for name in dictionary.get('Names', []):
            if len(name.split('/')) == 2:
                new_dictionary['Name'] = name
        return cls(client, new_dictionary, **kwargs)

    @classmethod
    def from_id(cls, client, id):
        return cls(client, client.inspect_container(id))

    @classmethod
    def create(cls, client, **options):
        response = client.create_container(**options)
        return cls.from_id(client, response['Id'])

    @property
    def flat_dictionary(self):
        self.inspect_if_not_inspected()
        return flatten(prepare_d(self.dictionary))

    @property
    def id(self):
        return self.dictionary['ID']

    @property
    def image(self):
        return self.dictionary['Image']

    @property
    def short_id(self):
        return self.id[:10]

    @property
    def name(self):
        return self.dictionary['Name'][1:]

    @property
    def name_without_project(self):
        return '_'.join(self.dictionary['Name'].split('_')[1:])

    @property
    def number(self):
        try:
            return int(self.name.split('_')[-1])
        except ValueError:
            return None

    @property
    def human_readable_ports(self):
        self.inspect_if_not_inspected()
        if not self.dictionary['NetworkSettings']['Ports']:
            return ''
        ports = []
        for private, public in list(self.dictionary['NetworkSettings']['Ports'].items()):
            if public:
                ports.append('%s->%s' % (public[0]['HostPort'], private))
        return ', '.join(ports)

    @property
    def human_readable_state(self):
        self.inspect_if_not_inspected()
        if self.dictionary['State']['Running']:
            if self.dictionary['State']['Ghost']:
                return 'Ghost'
            else:
                return 'Up'
        else:
            return 'Exit %s' % self.dictionary['State']['ExitCode']

    @property
    def human_readable_command(self):
        self.inspect_if_not_inspected()
        return ' '.join(self.dictionary['Config']['Cmd'])

    @property
    def environment(self):
        self.inspect_if_not_inspected()
        out = {}
        for var in self.dictionary.get('Config', {}).get('Env', []):
            k, v = var.split('=', 1)
            out[k] = v
        return out

    @property
    def is_running(self):
        self.inspect_if_not_inspected()
        return self.dictionary['State']['Running']

    def start(self, **options):
        return self.client.start(self.id, **options)

    def stop(self, **options):
        return self.client.stop(self.id, **options)

    def kill(self):
        return self.client.kill(self.id)

    def remove(self):
        return self.client.remove_container(self.id)

    def inspect_if_not_inspected(self):
        if not self.has_been_inspected:
            self.inspect()

    def wait(self):
        return self.client.wait(self.id)

    def logs(self, *args, **kwargs):
        return self.client.logs(self.id, *args, **kwargs)

    def inspect(self):
        self.dictionary = self.client.inspect_container(self.id)
        return self.dictionary

    def links(self):
        links = []
        for container in self.client.containers():
            for name in container['Names']:
                bits = name.split('/')
                if len(bits) > 2 and bits[1] == self.name:
                    links.append(bits[2])
        return links

    def attach(self, *args, **kwargs):
        return self.client.attach(self.id, *args, **kwargs)

    def attach_socket(self, **kwargs):
        return self.client.attach_socket(self.id, **kwargs)

    def __repr__(self):
        return '<Container: %s>' % self.name

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return self.id == other.id
