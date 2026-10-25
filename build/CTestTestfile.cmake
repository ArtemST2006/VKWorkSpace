# CMake generated Testfile for 
# Source directory: /home/artem/artem/projects/vkworkspace/msgfmt
# Build directory: /home/artem/artem/projects/vkworkspace/build
# 
# This file includes the relevant testing commands required for 
# testing this directory and lists subdirectories to be tested as well.
add_test([=[msgfmt_tests]=] "/home/artem/artem/projects/vkworkspace/build/msgfmt_tests")
set_tests_properties([=[msgfmt_tests]=] PROPERTIES  _BACKTRACE_TRIPLES "/home/artem/artem/projects/vkworkspace/msgfmt/CMakeLists.txt;24;add_test;/home/artem/artem/projects/vkworkspace/msgfmt/CMakeLists.txt;0;")
add_test([=[msgfmt_cli]=] "/usr/bin/cmake" "-DMSGFMT=/home/artem/artem/projects/vkworkspace/build/msgfmt" "-DTEST_DIR=/home/artem/artem/projects/vkworkspace/build" "-P" "/home/artem/artem/projects/vkworkspace/msgfmt/tests/cli.cmake")
set_tests_properties([=[msgfmt_cli]=] PROPERTIES  _BACKTRACE_TRIPLES "/home/artem/artem/projects/vkworkspace/msgfmt/CMakeLists.txt;25;add_test;/home/artem/artem/projects/vkworkspace/msgfmt/CMakeLists.txt;0;")
