cmake_minimum_required(VERSION 3.22.1)

project(anpfuel_monocypher_ed25519 C)

add_library(
    monocypher_ed25519
    SHARED
    ed25519_bridge.c
    monocypher/monocypher.c
    monocypher/monocypher-ed25519.c
)

target_include_directories(monocypher_ed25519 PRIVATE ${CMAKE_CURRENT_SOURCE_DIR})

set_target_properties(
    monocypher_ed25519
    PROPERTIES
    C_STANDARD 99
    C_STANDARD_REQUIRED YES
)

# Keep 16 KB page alignment consistent with the MAP-14 engine gate so the
# small verifier loads on 16 KB devices without a separate waiver.
if(ANDROID)
    target_link_options(monocypher_ed25519 PRIVATE "-Wl,-z,max-page-size=16384")
endif()
