from __future__ import absolute_import
from __future__ import unicode_literals

import six
from docker.errors import APIError

from compose.parallel import parallel_execute
from compose.parallel import parallel_execute_iter
from compose.parallel import UpstreamError


web = 'web'
db = 'db'
data_volume = 'data_volume'
cache = 'cache'

objects = [web, db, data_volume, cache]

deps = {
    web: [db, cache],
    db: [data_volume],
    data_volume: [],
    cache: [],
}


def get_deps(obj):
    return deps[obj]


def test_parallel_execute():
    results, errors = parallel_execute(
        objects=[1, 2, 3, 4, 5],
        func=lambda x: x * 2,
        get_name=six.text_type,
        msg="Doubling",
    )

    assert sorted(results) == [2, 4, 6, 8, 10]
    assert errors == {}


def test_parallel_execute_with_deps():
    log = []

    def process(x):
        log.append(x)

    parallel_execute(
        objects=objects,
        func=process,
        get_name=lambda obj: obj,
        msg="Processing",
        get_deps=get_deps,
    )

    assert sorted(log) == sorted(objects)

    assert log.index(data_volume) < log.index(db)
    assert log.index(db) < log.index(web)
    assert log.index(cache) < log.index(web)


def test_parallel_execute_with_upstream_errors():
    log = []

    def process(x):
        if x is data_volume:
            raise APIError(None, None, "Something went wrong")
        log.append(x)

    parallel_execute(
        objects=objects,
        func=process,
        get_name=lambda obj: obj,
        msg="Processing",
        get_deps=get_deps,
    )

    assert log == [cache]

    events = [
        (obj, result, type(exception))
        for obj, result, exception
        in parallel_execute_iter(objects, process, get_deps)
    ]

    assert (cache, None, type(None)) in events
    assert (data_volume, None, APIError) in events
    assert (db, None, UpstreamError) in events
    assert (web, None, UpstreamError) in events
