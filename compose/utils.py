import json
import hashlib
import sys


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    if (sys.version_info > (3, 0)):
        dump = dump.encode('utf-8')
    h.update(dump)
    return h.hexdigest()
