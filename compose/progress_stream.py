import json

import six

from compose import utils


class StreamOutputError(Exception):
    pass


def stream_output(output, stream):
    is_terminal = hasattr(stream, 'isatty') and stream.isatty()
    stream = utils.get_output_stream(stream)
    all_events = []
    lines = {}
    diff = 0

    for chunk in output:
        if six.PY3:
            chunk = chunk.decode('utf-8')
        event = json.loads(chunk)
        all_events.append(event)

        if 'progress' in event or 'progressDetail' in event:
            image_id = event.get('id')
            if not image_id:
                continue

            if image_id in lines:
                diff = len(lines) - lines[image_id]
            else:
                lines[image_id] = len(lines)
                stream.write("\n")
                diff = 0

            if is_terminal:
                # move cursor up `diff` rows
                stream.write("%c[%dA" % (27, diff))

        print_output_event(event, stream, is_terminal)

        if 'id' in event and is_terminal:
            # move cursor back down
            stream.write("%c[%dB" % (27, diff))

        stream.flush()

    return all_events


def print_output_event(event, stream, is_terminal):
    if 'errorDetail' in event:
        raise StreamOutputError(event['errorDetail']['message'])

    terminator = ''

    if is_terminal and 'stream' not in event:
        # erase current line
        stream.write("%c[2K\r" % 27)
        terminator = "\r"
    elif 'progressDetail' in event:
        return

    if 'time' in event:
        stream.write("[%s] " % event['time'])

    if 'id' in event:
        stream.write("%s: " % event['id'])

    if 'from' in event:
        stream.write("(from %s) " % event['from'])

    status = event.get('status', '')

    if 'progress' in event:
        stream.write("%s %s%s" % (status, event['progress'], terminator))
    elif 'progressDetail' in event:
        detail = event['progressDetail']
        total = detail.get('total')
        if 'current' in detail and total:
            percentage = float(detail['current']) / float(total) * 100
            stream.write('%s (%.1f%%)%s' % (status, percentage, terminator))
        else:
            stream.write('%s%s' % (status, terminator))
    elif 'stream' in event:
        stream.write("%s%s" % (event['stream'], terminator))
    else:
        stream.write("%s%s\n" % (status, terminator))
