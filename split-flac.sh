#!/bin/bash


FILE=$1

SEGMENTS=$(ffmpeg -i $FILE -af silencedetect=d=0.5 -f null - |& grep -Po "silence_(start|end): \d+\.\d+" | cut -f 2 -d ' ' | paste -sd ',')

ffmpeg -i $FILE -f segment -segment_times $SEGMENTS -reset_timestamps 1 -map 0:a -c flac output_%03d.flac
