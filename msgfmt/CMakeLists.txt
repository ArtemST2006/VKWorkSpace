cmake_minimum_required(VERSION 3.20)
project(msgfmt LANGUAGES CXX)

add_library(msgfmt_lib src/msgfmt.cpp)
target_include_directories(msgfmt_lib PUBLIC include)
target_compile_features(msgfmt_lib PUBLIC cxx_std_17)

add_executable(msgfmt src/main.cpp)
target_link_libraries(msgfmt PRIVATE msgfmt_lib)

set_target_properties(msgfmt_lib msgfmt PROPERTIES
    CXX_STANDARD 17
    CXX_STANDARD_REQUIRED YES
    CXX_EXTENSIONS NO)

include(CTest)
if(BUILD_TESTING)
    add_executable(msgfmt_tests tests/test_msgfmt.cpp)
    target_link_libraries(msgfmt_tests PRIVATE msgfmt_lib)
    set_target_properties(msgfmt_tests PROPERTIES
        CXX_STANDARD 17
        CXX_STANDARD_REQUIRED YES
        CXX_EXTENSIONS NO)
    add_test(NAME msgfmt_tests COMMAND msgfmt_tests)
    add_test(NAME msgfmt_cli COMMAND ${CMAKE_COMMAND}
        -DMSGFMT=$<TARGET_FILE:msgfmt>
        -DTEST_DIR=${CMAKE_CURRENT_BINARY_DIR}
        -P ${CMAKE_CURRENT_SOURCE_DIR}/tests/cli.cmake)
endif()

if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    foreach(target msgfmt_lib msgfmt)
        target_compile_options(${target} PRIVATE -Wall -Wextra -Wpedantic)
    endforeach()
    if(BUILD_TESTING)
        target_compile_options(msgfmt_tests PRIVATE -Wall -Wextra -Wpedantic)
    endif()
endif()
