#!/usr/bin/env python3
"""KUBECTL_EXTERNAL_DIFF normalizer for the R2 zero-diff proof.

It removes only server-managed metadata and client-apply bookkeeping
(creationTimestamp, generation, resourceVersion, uid, managedFields, status,
last-applied-configuration, deployment revision and manual restart timestamps).
Spec, labels, desired annotations, data and image pull policies are compared.
"""
import difflib
import os
import sys

import yaml

SERVER_MANAGED = {
    "creationTimestamp",
    "generation",
    "resourceVersion",
    "uid",
    "managedFields",
    "selfLink",
    "status",
}
SERVER_ANNOTATIONS = {
    "kubectl.kubernetes.io/last-applied-configuration",
    "deployment.kubernetes.io/revision",
    "kubectl.kubernetes.io/restartedAt",
}


def clean(node):
    if isinstance(node, dict):
        for key in SERVER_MANAGED:
            node.pop(key, None)
        annotations = node.get("annotations")
        if isinstance(annotations, dict):
            for key in SERVER_ANNOTATIONS:
                annotations.pop(key, None)
            if not annotations:
                node.pop("annotations", None)
        for value in node.values():
            clean(value)
    elif isinstance(node, list):
        for value in node:
            clean(value)
    return node


def normalized(path):
    with open(path, encoding="utf-8") as handle:
        docs = [doc for doc in yaml.safe_load_all(handle) if doc is not None]
    clean(docs)
    return yaml.safe_dump_all(docs, sort_keys=True, default_flow_style=False)


def diff_files(live, merged, label):
    return list(
        difflib.unified_diff(
            normalized(live).splitlines(keepends=True),
            normalized(merged).splitlines(keepends=True),
            fromfile=f"live/{label}",
            tofile=f"merged/{label}",
        )
    )


def main():
    args = [arg for arg in sys.argv[1:] if not arg.startswith("-")]
    if len(args) < 2:
        sys.exit(2)
    live, merged = args[-2], args[-1]
    output = []
    if os.path.isdir(live) and os.path.isdir(merged):
        files = []
        for root, _, names in os.walk(live):
            for name in names:
                files.append(os.path.relpath(os.path.join(root, name), live))
        for relative in sorted(files):
            source = os.path.join(live, relative)
            target = os.path.join(merged, relative)
            if os.path.exists(target):
                output.extend(diff_files(source, target, relative))
            else:
                output.append(f"Only in live: {relative}\n")
    else:
        output.extend(diff_files(live, merged, os.path.basename(live)))
    if not output:
        sys.exit(0)
    sys.stdout.writelines(output)
    sys.exit(1)


if __name__ == "__main__":
    main()
