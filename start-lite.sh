#!/bin/bash
cd /home/claw/WeKnora
set -a
. ./.env.lite
set +a
exec ./WeKnora-lite
