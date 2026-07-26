#!/bin/sh
# A long-lived app for the lifecycle scenario: print one line (so logs have content),
# then block so the container stays up until zc/zcr stops it.
echo "sleeper up"
exec sleep 300
