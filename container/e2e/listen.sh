#!/bin/sh
# The producer: listen on the published port (5432) and an unpublished one (9999). Each
# replies with a tag so a prober can tell which port answered. Re-listen in a loop so
# several probes in a row all get served.
while true; do echo P5432 | nc -l -p 5432; done &
while true; do echo P9999 | nc -l -p 9999; done &
echo "listener up on 5432 and 9999"
wait
