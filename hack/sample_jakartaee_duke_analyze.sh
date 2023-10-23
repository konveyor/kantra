#!/bin/sh
#
export PATH=$PATH:./
mkdir -p tmp
# Go into ./tmp and add an out dir and our sample code with correct branch
cd tmp
mkdir -p out
git clone https://github.com/ivargrimstad/jakartaee-duke.git
cd jakartaee-duke
git checkout start-tutorial
cd ../..
# Backing out to current working directory
time kantra analyze -i $PWD/tmp/jakartaee-duke -t "jakarta-ee9+" -o $PWD/tmp/out


