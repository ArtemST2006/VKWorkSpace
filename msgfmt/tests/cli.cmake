set(input "${TEST_DIR}/msgfmt-cli-input.txt")
file(WRITE "${input}" "**важно**")
execute_process(COMMAND "${MSGFMT}" INPUT_FILE "${input}"
    RESULT_VARIABLE code OUTPUT_VARIABLE output ERROR_VARIABLE diagnostic)
set(expected "{\"text\":\"важно\",\"ranges\":[{\"style\":\"bold\",\"offset\":0,\"length\":5,\"url\":\"\"}]}\n")
if(NOT code STREQUAL "0" OR NOT output STREQUAL expected OR NOT diagnostic STREQUAL "")
    message(FATAL_ERROR "parse CLI: code=${code}, stdout=${output}, stderr=${diagnostic}")
endif()

file(WRITE "${input}" "2 * 3")
execute_process(COMMAND "${MSGFMT}" escape INPUT_FILE "${input}"
    RESULT_VARIABLE code OUTPUT_VARIABLE output ERROR_VARIABLE diagnostic)
set(expected "2 \\* 3")
if(NOT code STREQUAL "0" OR NOT output STREQUAL expected OR NOT diagnostic STREQUAL "")
    message(FATAL_ERROR "escape CLI: code=${code}, stdout=${output}, stderr=${diagnostic}")
endif()

string(ASCII 255 invalid_byte)
file(WRITE "${input}" "${invalid_byte}")
execute_process(COMMAND "${MSGFMT}" INPUT_FILE "${input}"
    RESULT_VARIABLE code OUTPUT_VARIABLE output ERROR_VARIABLE diagnostic)
set(expected "{\"error\":\"invalid_utf8\"}\n")
if(NOT code STREQUAL "2" OR NOT output STREQUAL expected OR NOT diagnostic STREQUAL "")
    message(FATAL_ERROR "invalid UTF-8 CLI: code=${code}, stdout=${output}, stderr=${diagnostic}")
endif()
