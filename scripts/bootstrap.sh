#!/bin/bash
CURDIR=$(cd $(dirname $0); pwd)
BinaryName=git-ferry
echo "$CURDIR/bin/${BinaryName}"
exec $CURDIR/bin/${BinaryName}