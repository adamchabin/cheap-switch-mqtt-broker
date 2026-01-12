#!/bin/bash

#source ./.env;
export $(grep -v '^#' .env | xargs)

./build.sh && ./poe-mqtt-bridge