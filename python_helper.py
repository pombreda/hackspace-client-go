#!/usr/bin/env python
# Copyright 2013 The Swarming Authors. All rights reserved.
# Use of this source code is governed by the Apache v2.0 license that can be
# found in the LICENSE file.

import sys
import json

sys.path.append("/s/swarming/client")

import isolate_format


def load_isolate_as_config(args, stdin):
  # Reads isolate content from stdin .
  isolate_dir, = args
  # file_comment is "" .
  out = isolate_format.load_isolate_as_config(isolate_dir, eval(stdin.read()), "")

  # we pass output to Go as json

  def convKey(valuesOfKeys):
    return [{
      'Value': '' if k is None else k[0],
      'IsBound': not (k is None),
    } for k in valuesOfKeys]

  def convValue(v):
    r = -1 if v.read_only is None else v.read_only
    return {
      'ReadOnly' : r,
      'Files': v.files,
      'Command': v.command,
      'IsolateDir': v.isolate_dir,
    }

  return {
    'ConfigVariables' : out.config_variables,
    'ByConfig': list({'key': convKey(k), 'value': convValue(v)} for k, v in out._by_config.iteritems()),
  }

def eval_content(args, stdin):
  """Evaluates a python file and return the value defined in it.

  Used in practice for .isolate files.
  """
  assert not args
  content = stdin.read()
  globs = {'__builtins__': None}
  locs = {}
  try:
    value = eval(content, globs, locs)
  except TypeError as e:
    e.args = list(e.args) + [content]
    raise
  assert locs == {}, locs
  assert globs == {'__builtins__': None}, globs
  return value


if __name__ == "__main__":
    try:
        run = sys.argv[1]
        func = globals()[sys.argv[1]]
    except Exception:
        sys.stderr.write("bad arguments: %s\n" % sys.argv[1:])
        sys.exit(2)
    try:
        d = func(sys.argv[2:], sys.stdin)
        json.dump(d, sys.stdout)
        sys.exit(0)
    except Exception, e:
        sys.stderr.write("args: %s\n" % sys.argv)
        sys.stderr.write("raised %s\n" % e)
        raise
