from __future__ import absolute_import
from __future__ import unicode_literals

from compose import utils


class StreamOutputError(Exception):
    pass


def stream_output(output, stream):
    is_terminal = hasattr(stream, 'isatty') and stream.isatty()
    stream = utils.get_output_stream(stream)
    all_events = []
    lines = {}
    diff = 0

    for event in utils.json_stream(output):
        all_events.append(event)
        is_progress_event = 'progress' in event or 'progressDetail' in event

        if not is_progress_event:
            print_output_event(event, stream, is_terminal)
            stream.flush()
            continue

        if not is_terminal:
            continue

        # if it's a progress event and we have a terminal, then display the progress bars
        image_id = event.get('id')
        if not image_id:
            continue

        if image_id not in lines:
            lines[image_id] = len(lines)
            stream.write("\n")

        diff = len(lines) - lines[image_id]

        # move cursor up `diff` rows
        stream.write("%c[%dA" % (27, diff))

        print_output_event(event, stream, is_terminal)

        if 'id' in event:
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


def get_digest_from_pull(events):
    for event in events:
        status = event.get('status')
        if not status or 'Digest' not in status:
            continue

        _, digest = status.split(':', 1)
        return digest.strip()
    return None


def get_digest_from_push(events):
    for event in events:
        digest = event.get('aux', {}).get('Digest')
        if digest:
            return digest
    return None
