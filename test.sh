#!/bin/bash
set -xe

rm -r demo
./ll-builder create demo
cd demo
../ll-builder build
../ll-builder run
../ll-builder export --layer
ll-cli uninstall demo
ll-cli install *_binary.layer
ll-cli run demo
