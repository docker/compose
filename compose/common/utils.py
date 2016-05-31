from __future__ import absolute_import
from __future__ import unicode_literals

import codecs
import contextlib
import hashlib
import json
import json.decoder
import sys

import six

from . import signals


json_decoder = json.JSONDecoder()


def get_output_stream(stream):
    if six.PY3:
        return stream
    return codecs.getwriter('utf-8')(stream)


def stream_as_text(stream):
    """Given a stream of bytes or text, if any of the items in the stream
    are bytes convert them to text.

    This function can be removed once docker-py returns text streams instead
    of byte streams.
    """
    for data in stream:
        if not isinstance(data, six.text_type):
            data = data.decode('utf-8', 'replace')
        yield data


def line_splitter(buffer, separator=u'\n'):
    index = buffer.find(six.text_type(separator))
    if index == -1:
        return None
    return buffer[:index + 1], buffer[index + 1:]


def split_buffer(stream, splitter=None, decoder=lambda a: a):
    """Given a generator which yields strings and a splitter function,
    joins all input, splits on the separator and yields each chunk.

    Unlike string.split(), each chunk includes the trailing
    separator, except for the last one if none was found on the end
    of the input.
    """
    splitter = splitter or line_splitter
    buffered = six.text_type('')

    for data in stream_as_text(stream):
        buffered += data
        while True:
            buffer_split = splitter(buffered)
            if buffer_split is None:
                break

            item, buffered = buffer_split
            yield item

    if buffered:
        yield decoder(buffered)


def json_splitter(buffer):
    """Attempt to parse a json object from a buffer. If there is at least one
    object, return it and the rest of the buffer, otherwise return None.
    """
    try:
        obj, index = json_decoder.raw_decode(buffer)
        rest = buffer[json.decoder.WHITESPACE.match(buffer, index).end():]
        return obj, rest
    except ValueError:
        return None


def json_stream(stream):
    """Given a stream of text, return a stream of json objects.
    This handles streams which are inconsistently buffered (some entries may
    be newline delimited, and others are not).
    """
    return split_buffer(stream, json_splitter, json_decoder.decode)


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    h.update(dump.encode('utf8'))
    return h.hexdigest()


def microseconds_from_time_nano(time_nano):
    return int(time_nano % 1000000000 / 1000)


def build_string_dict(source_dict):
    return dict((k, str(v)) for k, v in source_dict.items())


@contextlib.contextmanager
def up_shutdown_context(project, service_names, timeout, detached, is_cli_call=True):
    if detached:
        yield
        return

    signals.set_signal_handler_to_shutdown()
    try:
        try:
            yield
        except signals.ShutdownException:
            if is_cli_call:
                print("Gracefully stopping... (press Ctrl+C again to force)")
            project.stop(service_names=service_names, timeout=timeout)
    except signals.ShutdownException:
        project.kill(service_names=service_names)
        if is_cli_call:
            sys.exit(2)


def convergence_strategy_from_opts(no_recreate=None,
                                   force_recreate=None,
                                   exception_class=Exception,
                                   message="force_recreate and no_"
                                           "recreate cannot be combined.",
                                   force_recreate_strategy=None,
                                   no_recreate_strategy=None,
                                   alternative_strategy=None):
    print(no_recreate, force_recreate)
    if force_recreate and no_recreate:
        raise exception_class(message)

    if force_recreate:
        return force_recreate_strategy

    if no_recreate:
        return no_recreate_strategy

    return alternative_strategy


def build_action_from_opts(build=None, no_build=None, exception_class=Exception,
                           message="build and no_build can not be combined.",
                           build_action=None, no_build_action=None,
                           alternative_action=None):
    if build and no_build:
        raise exception_class(message)

    if build:
        return build_action

    if no_build:
        return no_build_action

    return alternative_action


def image_type_from_opt(flag, value,
                        image_type_class=None,
                        exception_class=Exception):
    if not value:
        return image_type_class.none
    try:
        return image_type_class[value]
    except KeyError:
        raise exception_class("%s flag must be one of: all, local" % flag)


def prepare_container_opts(cmd, detached_mode, os_environment, entry_point,
                           remove_after_run, run_as_user_or_uid, name,
                           container_working_dir, service_ports, publish_ports):
    container_options = {
        'command': cmd,
        'tty': not detached_mode,
        'stdin_open': not detached_mode,
        'detach': detached_mode,
        'environment': os_environment,
        'entry_point': entry_point,
        'restart': not remove_after_run,
        'user': run_as_user_or_uid,
        'name': name,
        'working_dir': container_working_dir,
    }
    if not service_ports:
        container_options.update(ports=[])
    if publish_ports:
        container_options.update(ports=publish_ports)

    return container_options


def run_one_off(self, container_opts, service, no_deps,
                detached_mode, remove_after_run, tty, pty_object, strategy):
    if not no_deps:
        deps = service.get_dependency_names()
        self.up(service_names=deps,
                start_deps=True,
                strategy=strategy)
    self.initialize()
    container = service.create_container(
        quiet=True,
        one_off=True,
        **container_opts)

    if detached_mode:
        service.start_container(container)
        return True

    def remove_container(force=False):
        if remove_after_run:
            self.client.remove_container(container.id,
                                         force=True)

    signals.set_signal_handler_to_shutdown()
    try:
        try:
            operation = pty_object.RunOperation(
                self.client,
                container.id,
                interactive=not tty,
                logs=False,
            )
            pty = pty_object.PseudoTerminal(self.client, operation)
            sockets = pty.sockets()
            service.start_container(container)
            pty.start(sockets)
            container.wait()
            return True
        except signals.ShutdownException:
            self.client.stop(container.id)
    except signals.ShutdownException:
        self.client.kill(container.id)
        remove_container(force=True)
        return False
