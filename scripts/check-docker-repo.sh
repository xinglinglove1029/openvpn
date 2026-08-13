#!/bin/bash
for d in resolute noble plucky; do
  echo -n "$d: "
  curl -sI --max-time 8 "https://download.docker.com/linux/ubuntu/dists/$d/Release" | head -1
done
