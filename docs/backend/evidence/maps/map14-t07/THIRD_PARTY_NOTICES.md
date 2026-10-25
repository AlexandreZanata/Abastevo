# Third-party notices

Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827`
Artifact: pinned CoMaps SDK release candidate (paths omitted for reproducibility)
Source notices; resource admission is recorded in the packaged resources.json.

The core is Apache-2.0; the submodule licenses below are the exact
pinned sources. The full license files are copied under `licenses/`.
This bundle is a technical inventory, not a legal opinion.

## Resolved items

- `3party/glm`: **MIT**. Dual Happy Bunny (modified MIT) / MIT; MIT selected.
- `3party/just_gtfs`: **MIT**. MIT is present in ancestor 083d98e (2020-03-28), with copyright 2020 mesozoic-drones; its grant covers the pinned descendant code. The fork omitted the file; full terms are retained.
  Upstream evidence: https://github.com/comaps/just_gtfs/blob/master/LICENSE.MIT (`b5b1df6f5e26c7698d506a1bd0b8cfb5ffe2d64f`).
- `3party/monocypher`: **BSD-2-Clause**. Pinned source offers BSD-2-Clause or CC0; BSD-2-Clause selected.
- `3party/GL`: **MIT**. Khronos MIT-style header.
- `3party/agg`: **LicenseRef-AGG**. Anti-Grain Geometry permissive header; notice retained.
- `3party/ankerl`: **MIT**. MIT header.
- `3party/kdtree++`: **Artistic-2.0**. Headers state Artistic-2.0; the pinned tree omits the referenced COPYING, so the full license is retained in the Abastevo notices.
- `3party/minizip`: **Zlib AND Info-ZIP**. MiniZip64_info.txt supplies the zlib terms; unzip.c references Info-ZIP, whose full license is retained in the Abastevo notices.
- `3party/opening_hours`: **MIT**. MIT header.
- `3party/robust`: **LicenseRef-PublicDomain**. Public-domain header.
- `3party/skarupke`: **BSL-1.0**. Boost Software License 1.0 header.
- `3party/stb_image`: **MIT**. Dual MIT/public-domain; MIT selected.
- `3party/vulkan_wrapper`: **Apache-2.0**. Apache-2.0 header.

## Components

| Component | Kind | Commit | Status | Scope | License evidence |
| --- | --- | --- | --- | --- | --- |
| `3party/CMake-MetalShaderSupport` | submodule | `84209c32e50966ee4dca66f7b7eebacd934c87fb` | PRESENT | OWN | `LICENCE.txt` `6e9c2fd4f0c8…` |
| `3party/Vulkan-Headers` | submodule | `f69f0433bae0b30598380ef0420b9d2d02dbac4d` | PRESENT | OWN | `LICENSE.md` `ac24e5ea920e…` |
| `3party/boost` | submodule | `ab7968a0bbcf574a7859240d1d8443f58ed6f6cf` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/accumulators` | submodule | `9d9e5dae2202660f57e2dc91efb620aa001525b3` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/algorithm` | submodule | `32c5a6327cfdca5d41ce0f1d8849b811886daa2f` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/align` | submodule | `5ad7df63cd792fbdb801d600b93cad1a432f0151` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/any` | submodule | `aee53090c04bd0a263480fae25fddddcef54ca82` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/array` | submodule | `23f6b27c0d9916b9932baac898ae3009817a9153` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/asio` | submodule | `e65367991cb5fbdb8a7cf218ae38f69ca9a0a9f5` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/assert` | submodule | `447e0b3a331930f8708ade0e42683d12de9dfbc3` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/assign` | submodule | `ababd47970e8a5fa1bebc8ccad526c4f25bd867a` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/atomic` | submodule | `5bbcce0f6e855dc4009e2e6977c62e0520c39573` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/beast` | submodule | `98b8be489fa7a74753a724d8d0772d90bcbed0fc` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/bimap` | submodule | `6865e94cc56a33bb8b162bf8d62cfe620b06d2b6` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/bind` | submodule | `9fbfdcb3577e9427815d4f8cc25b3a25d5b9696b` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/callable_traits` | submodule | `2a56a3a2496cdb66496f844db55085dd992d5e49` | PRESENT | OWN | `LICENSE.md` `754f0d4ecb5f…` |
| `3party/boost/libs/charconv` | submodule | `ecdca0865d4d8fcfbf8e5b48914f2b5711c418ad` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/chrono` | submodule | `ee0d6d543a37d9b7243682549e9ae359eb89daa9` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/circular_buffer` | submodule | `a08a5b55ee82e0c2487523471379ac53a23935dc` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/cobalt` | submodule | `0b813affb41e0dd2a42c3c34033f4ae68431b3e9` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/compat` | submodule | `bba8d5f1011afc61b9fbaf0e96417d72cb09f8ec` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/compatibility` | submodule | `d0caac5c346f7e24b4f8ec1e55110119492b64bd` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/compute` | submodule | `36350b7de849300bd3d72a05d8bf890ca405a014` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/concept_check` | submodule | `37c9bddf0bdefaaae0ca5852c1a153d9fc43f278` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/config` | submodule | `11385ec21012926e15a612e3bf9f9a71403c1e5b` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/container` | submodule | `6e697d796897b32b471b4f0740dcaa03d8ee57cc` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/container_hash` | submodule | `6d214eb776456bf17fbee20780a034a23438084f` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/context` | submodule | `1bde50e400547e29336afe7ea0cd693d8c884fb6` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/contract` | submodule | `eca93d24b5d3bb909ed64c12b5feb5296c5cc070` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/conversion` | submodule | `9f285ef0c43c101e49b37bf5e6085e8d635887dc` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/convert` | submodule | `1dd2ca23cb74991d420ea85b9b764f0ac76367c1` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/core` | submodule | `083b41c17e34f1fc9b43ab796b40d0d8bece685c` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/coroutine` | submodule | `1e1347c0b1910b9310ec1719edad8b0bf2fd03c8` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/coroutine2` | submodule | `d7e1c1c4abcf8c1e90097279e485edea0b253a80` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/crc` | submodule | `e3b1e56890a701ded5e66929579fb7fa62ac6bcc` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/date_time` | submodule | `85e637cb325208c2af9af791c3a1948b4888c6cd` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/describe` | submodule | `50719b212349f3d1268285c586331584d3dbfeb5` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/detail` | submodule | `9c3a0022b25d3e483f9100cc363bc93a72fd900a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/dll` | submodule | `6c60dde50bf67138c90cc84938111866813feaff` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/dynamic_bitset` | submodule | `8e20aa1462bf6dcadc338835df529a6d568431b1` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/endian` | submodule | `c9b436e5dfce85e8ae365e5aabbb872dd35c29eb` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/exception` | submodule | `b9170a02f102250b308c9f94ed6593c5f30eab39` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/fiber` | submodule | `88b8304e78dfab4baee5d6beca09938d2826061a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/filesystem` | submodule | `a0c8edba38a4d31b449fcf7b7ada455977342596` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/flyweight` | submodule | `e9f3a0a005eca8baa377f13423107b35985d9119` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/foreach` | submodule | `cc2f75ae30492b9de69b3b692f5c59afcb7dea5e` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/format` | submodule | `78ef371d2d90462671b90c3af407fae07820b193` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/function` | submodule | `28b88d07bb4807445462c3f5dab0efde6f532d32` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/function_types` | submodule | `895335874d67987ada0d8bf6ca1725e70642ed49` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/functional` | submodule | `6a573e4b8333ee63ee62ce95558c3667348db233` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/fusion` | submodule | `7d4c03fa032299f2d46149b7b3136c9fd43e4f81` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/geometry` | submodule | `f82eb32da79641697583a43be9988d7517d8553c` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/gil` | submodule | `8994c2ff8ed8b88e0ffef9658a4c603c4a001fad` | PRESENT | OWN | `LICENSE.txt` `c9bff7573892…` |
| `3party/boost/libs/graph` | submodule | `a649be53bd90ab3365c6c0c44414c80907cfd8a1` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/graph_parallel` | submodule | `5520a5617d2763c48a06a4ff277ad76e665c7cf3` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/hana` | submodule | `e66cd6bc3fdba37b31440e013d026a10e26ca486` | PRESENT | OWN | `LICENSE.md` `367ef0a8422d…` |
| `3party/boost/libs/headers` | submodule | `0456900fadde4b07c84760eadea4ccc9f948fe28` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/heap` | submodule | `dc2f19f8815cbe0654df61bfc5f31ad8b06fc883` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/histogram` | submodule | `a29729e6682652800b06dc8aded29bbeb4acea61` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/hof` | submodule | `63ac019fca325038bfb4f56db17fbb281d2b5f49` | PRESENT | OWN | `LICENSE.md` `859bddbbef21…` |
| `3party/boost/libs/icl` | submodule | `2741c3a1b7b79f7326360c26c3e1588b385c9e53` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/integer` | submodule | `dea8e3445dc3ca29201498260307138b9460a70c` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/interprocess` | submodule | `8e4caa9fca8ff92b739b570f399f4407da0ecc2f` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/intrusive` | submodule | `07ba0e376177409c396c109807c13b7181a98ebe` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/io` | submodule | `342e4c6d10d586058818daa84201a2d301357a53` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/iostreams` | submodule | `6be38c3449a64b15213c6a490d357c9a2ac2470b` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/iterator` | submodule | `4f7219965a399051bb0d8088ea4ab3929b1ac3f2` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/json` | submodule | `9f85ed6d62ff91c6dc4fc30e3a20e9049ec67585` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/lambda` | submodule | `1555e9c7fff978d94359fc3f6312187e1e2647d5` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/lambda2` | submodule | `a17ffc25d303635fcc44dfa97d3e3c4b04e9db5e` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/leaf` | submodule | `ed8f9cd32f4fde695d497502f696f6f861b68559` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/lexical_cast` | submodule | `02e5821ab32c45fad719829e9644e5d681c9ba0b` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/local_function` | submodule | `258aeaa6c1a9618c02ca4e4af2890e70833035f0` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/locale` | submodule | `5edb7eb4f4848546d3d285598374b5d37daae387` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/lockfree` | submodule | `fdd4d0632dd0904f6e9c656c45397fe8ef985bc9` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/log` | submodule | `2d932ddcb5b05a9b2cc0a9df2d8a6ff88883f2b3` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/logic` | submodule | `145778490c2d332c1411df6a5274a4b53ec3e091` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/math` | submodule | `434a017db4c7abc115d8a8ae08a446b70feb34f2` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/metaparse` | submodule | `0ef448c1a7ce22b5de514f9cd504c323c28f4379` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/move` | submodule | `7c01072629d83a7b54c99de70ef535d699ebd200` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/mp11` | submodule | `863d8b8d2b20f2acd0b5870f23e553df9ce90e6c` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/mpi` | submodule | `347595c77379ac5cbfc6e4474fe315398fef355e` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/mpl` | submodule | `b440c45c2810acbddc917db057f2e5194da1a199` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/msm` | submodule | `f607d5154df4a5cc7f5ae014f0796f0b21b8bfc4` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/multi_array` | submodule | `0c5348bef71b890c4bd06eff1ee5ebda69e7b27a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/multi_index` | submodule | `d702a48363997f65fb717346f6694e0703902f8a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/multiprecision` | submodule | `51e01210078bae160ebc9fc4ede9281928d5b3c1` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/mysql` | submodule | `6397d04ff55741a29ca2c93975e26226cf773a7d` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/nowide` | submodule | `72429c3f48a1e70c08b5bd922ddebf7acddcbfb4` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/numeric/conversion` | submodule | `50a1eae942effb0a9b90724323ef8f2a67e7984a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/numeric/interval` | submodule | `2eda7413ac16dd4158005446438daf8a7e435dd9` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/numeric/odeint` | submodule | `1f4e47dfb395461a5c00d23b3728062f8290dfe6` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/numeric/ublas` | submodule | `9208885085dbfa8ec82c4023ff7d72741785e27a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/optional` | submodule | `c60db27762ff9cc16529e069c3c15f2fa898f994` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/outcome` | submodule | `1e3270d8106a59ac0b9c6e317fbfc836d2c66427` | PRESENT | OWN | `LICENSE` `2ab1c8959787…` |
| `3party/boost/libs/parameter` | submodule | `c07f2b8d37ded87f6f9d5bac867550f6e61282c1` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/parameter_python` | submodule | `1f7f9ce9930119f0dda7dcd5e1ec3b5ed7c6b091` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/pfr` | submodule | `449bf360f7d7a945639710b0fbb670ffde70e772` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/phoenix` | submodule | `8913607a3788cb82d48ed461ea59c919b7bad3df` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/poly_collection` | submodule | `0b8bfc4cff012d0f23049fc5a0009ac4abadceb4` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/polygon` | submodule | `8ba35b57c1436c4b36f7544aadd78c2b24acc7db` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/pool` | submodule | `ec7da07ed13e0c61e50d945b574a12ae7ec83cf4` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/predef` | submodule | `0fdfb49c3a6789e50169a44e88a07cc889001106` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/preprocessor` | submodule | `c4ea7e40d365ba28faecef8917d5c3f1e0121bf9` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/process` | submodule | `406cd3ecf36c19b94b4376241a4b3b43d1eae308` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/program_options` | submodule | `27f927694547277f845ae855e80b96be252f826c` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/property_map` | submodule | `e3a3c3655f4118fd15a02d8315f86a48db7390fd` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/property_map_parallel` | submodule | `a2f90e9660e4e7e012c0b54a1338d8e69fb71906` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/property_tree` | submodule | `8080ecd14f2d952a4bb7ae869fc0f54f54f5a31f` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/proto` | submodule | `7f924934689b940f3a72212ab0f714ec6fd6e34b` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/ptr_container` | submodule | `cf94735c87e073b33e54887a3d5e33598177ee1a` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/python` | submodule | `47d5bc76f69e20625214381c930a2fad5765e2b3` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/qvm` | submodule | `68f960dc43b52c0cce7b2e2c5f3d8c53ed3f454d` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/random` | submodule | `09594050246adcf944be2b2c8dd2b9901083de79` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/range` | submodule | `2bb6b636796f7b008196888613f51f5bb347c77d` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/ratio` | submodule | `d5b33caa7d564be9be6d962b18659b7741d764ac` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/rational` | submodule | `564623136417068916495e2b24737054d607347c` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/redis` | submodule | `b6e1280075fa0f2c71af3c7b89ef2717aa922557` | PRESENT | OWN | `LICENSE.txt` `c9bff7573892…` |
| `3party/boost/libs/regex` | submodule | `4cbcd3078e6ae10d05124379623a1bf03fcb9350` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/safe_numerics` | submodule | `777e0be5ec763d0333a717c5e421a4f7c5e5bdc9` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/scope` | submodule | `8321450aca6fc6a075225ba225036e585ce1716b` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/scope_exit` | submodule | `60baaae454b2da887a31cf939e22015b6263c9e4` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/serialization` | submodule | `a20c4d97c37e5f437c8ba78f296830edb79cff9e` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/signals2` | submodule | `4d994bbed820882059c7842ba5e230a26d68f2f0` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/smart_ptr` | submodule | `3ef8cfd02f9cb3e0486c26f35ed4c27c6f26bd5f` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/sort` | submodule | `d12ad000fc63d9c21e299c9ef420ccf85cba8548` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/spirit` | submodule | `43112c3f055fa109f20b8e8e6e50d0a016da742a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/stacktrace` | submodule | `351b03d5220510a1b6738cc378659b6d85c8a4da` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/statechart` | submodule | `586445b824c5cf0e7e6ce4ff2df620fda5d0f0d7` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/static_assert` | submodule | `ba72d3340f3dc6e773868107f35902292f84b07e` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/static_string` | submodule | `ac84c4f40ec38f7a7237255738e3e5259aa4a1a8` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/stl_interfaces` | submodule | `0f422d58cb392674bace893d237a6ddd121d8c31` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/system` | submodule | `2fc720a1cbe51d588fecc4e0af9417bd769381d8` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/test` | submodule | `822dc24718a33316bcef78b08f6bf57ae20d9335` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/thread` | submodule | `aec18d337f41d8e3081ee65f5cf3b5090179ab0e` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/throw_exception` | submodule | `7c8ec2114bc1f9ab2a8afbd629b96fbdd5901294` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/timer` | submodule | `a701cafb6be0021cf8e154cab5fc5d680c863f06` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/tokenizer` | submodule | `90106f155bd72b62aaca0d9ad826f4132030dba0` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/tti` | submodule | `03734c54a51b6372ac3296d2fe5103b7360bcd3f` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/tuple` | submodule | `b67941dd7d03536a854b96f001954792311ab515` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/type_erasure` | submodule | `729f3e739abc65d36d46df470c5bab735599d042` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/type_index` | submodule | `e37bc99e85e85bcac420ac1d4c1a8a5bca280d47` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/type_traits` | submodule | `cc6fc3daa8e20c4a70b563d67ceb4c34d8c1c4d7` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/typeof` | submodule | `4bc9de322cd44373435540d4e6c8f207892fc9a0` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/units` | submodule | `45787015dd8c11653eb988260acf05c4af9d42e5` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/unordered` | submodule | `5e6b9291deb55567d41416af1e77c2516dc1250f` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/url` | submodule | `bbbef97a5b30cd6d11e0c0ad5994a70a136e35cb` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/utility` | submodule | `e0ecf927643b13c63dfd9a38741315ea8b845135` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/uuid` | submodule | `beeaab4758c2f48d0451c759a600b34116c20906` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/boost/libs/variant` | submodule | `b33ff4f811a27d4dc4a4c8a840310523de4df4f8` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/variant2` | submodule | `ff36c3aa1f82d1d9a55fd006dd47cf380fc517c0` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/vmd` | submodule | `34cad2c1a574d445812c7c2432d3a5a5c843b412` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/wave` | submodule | `c11757decf393f6f1cf46f3f70b0bf892cdda5ee` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/winapi` | submodule | `39396bd78254053f3137510478e8f956bd2b83d4` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/xpressive` | submodule | `4679fbd23f962bfa78d44acf5fa48f6f790642c0` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/libs/yap` | submodule | `ae49bf2744586e6bd6c0cedff4500a58a4386860` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/more` | submodule | `70ec2047cdc515fe2c6f026a589d5fa3b7414658` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/auto_index` | submodule | `d8be00789cbe3aae8f247bcedbfa01f39c1a4343` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/bcp` | submodule | `e0b9262f6ff4778973a29b601183f3f9367e69d2` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/boost_install` | submodule | `5afffa2881dd84134f60c7473292aea4241a17b6` | PRESENT | OWN | `LICENSE` `c6dbbf828498…`, `LICENSE_1_0.txt` `8d8291caf1ce…` |
| `3party/boost/tools/boostbook` | submodule | `faacd1f26a714a5e2ec7d5c67ef81da371a1de50` | PRESENT | OWN | `LICENSE` `2f59ce71dd2b…` |
| `3party/boost/tools/boostdep` | submodule | `8f9cb2ef559fbe760f6522ddb8f693c02b83c032` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/build` | submodule | `efbf6ce59b4023f7adc9be2fc3875d371c7f52c6` | PRESENT | OWN | `LICENSE.txt` `c9bff7573892…` |
| `3party/boost/tools/check_build` | submodule | `32ca50c8ccd35e0ff62301b53fddcc8945728c4a` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/cmake` | submodule | `7276e8055d0faab429b11983eccddc68fee075f1` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/docca` | submodule | `0ce4e198398dbb52f1de0029f4ed9b7d2bded273` | PRESENT | OWN | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/inspect` | submodule | `d18744cddbd4304f7b939131004571dbb301fa4f` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/litre` | submodule | `564d4d8d30b7e03ac5e25d78e14d2c19fa321c83` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/boost/tools/quickbook` | submodule | `12a390e624f26e2e3f615b10c9681ce5390b02b2` | PRESENT | INHERITED:3party/boost | `LICENSE_1_0.txt` `c9bff7573892…` |
| `3party/expat` | submodule | `a0dc7d5efacbe2b744211289c276e2b9168bd4ae` | PRESENT | OWN | `COPYING` `122f2c270004…` |
| `3party/fast_double_parser` | submodule | `252029ddac664370bdda3f0761675785d92a1573` | PRESENT | OWN | `LICENSE` `017bb2a1e3a6…` |
| `3party/fast_double_parser/benchmarks/dependencies/abseil-cpp` | submodule | `d936052d32a5b7ca08b0199a6724724aea432309` | PRESENT | OWN | `LICENSE` `c79a7fea0e3c…` |
| `3party/fast_double_parser/benchmarks/dependencies/double-conversion` | submodule | `f4cb2384efa55dee0e6652f8674b05763441ab09` | PRESENT | OWN | `COPYING` `4af93c12062c…`, `LICENSE` `4af93c12062c…` |
| `3party/fast_obj` | submodule | `42629f744269e004907a6fb4f16c6c7f69acc586` | PRESENT | OWN | `LICENSE` `ba17b3a33e85…` |
| `3party/freetype/freetype` | submodule | `97069edd163b66ce11e8152bee3055b2fa627e15` | PRESENT | OWN | `LICENSE.TXT` `2e3bbb7d7c5c…` |
| `3party/freetype/freetype/subprojects/dlg` | submodule | `72dfcc858c040c54a6a0b88fcb7e70ee186d3167` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/gflags` | submodule | `52e94563eba1968783864942fedf6e87e3c611f4` | PRESENT | OWN | `COPYING.txt` `90d1f3701813…` |
| `3party/glaze` | submodule | `5a58d7936ece70eeb64cc30aa82e9e83ae9c05bb` | PRESENT | OWN | `LICENSE` `5d49e66411a0…` |
| `3party/glfw` | submodule | `21fea01161e0d6b70c0c5c1f52dc8e7a7df14a50` | PRESENT | OWN | `LICENSE.md` `149704059b5d…` |
| `3party/glm` | submodule | `c834d06c2739f63e28e5665d4569dca9415e3891` | PRESENT | OWN | `LICENSE1.txt` `fcb758c7b2fb…`, `LICENSE2.txt` `4542a845f7f5…` |
| `3party/googletest` | submodule | `58d77fa8070e8cec2dc1ed015d66b454c8d78850` | PRESENT | OWN | `LICENSE` `9702de7e4117…` |
| `3party/harfbuzz/harfbuzz` | submodule | `788b469ad5e5f78611f665b6eb17afd0eb040f21` | PRESENT | OWN | `COPYING` `ba8f810f2455…` |
| `3party/icu/icu` | submodule | `7750081bda4b3bc1768ae03849ec70f67ea10625` | PRESENT | OWN | `LICENSE` `3ae033da0bfd…` |
| `3party/imgui/imgui` | submodule | `6982ce43f5b143c5dce5fab0ce07dd4867b705ae` | PRESENT | OWN | `LICENSE.txt` `0ebf8feb0615…` |
| `3party/jansson/jansson` | submodule | `96d160df90016066d04d493d1d69639474ba4f20` | PRESENT | OWN | `LICENSE` `150d90904cd8…` |
| `3party/just_gtfs` | submodule | `7516753825500f90ac2de6f18c256d5abec1ff33` | RESOLVED_UPSTREAM | RESOLVED_UPSTREAM | `just_gtfs-MIT.txt` `d552983597dc…` |
| `3party/protobuf/protobuf` | submodule | `9442c12e866b56369f1c53abea1261259c924a19` | PRESENT | OWN | `LICENSE` `af67c58de2e1…` |
| `3party/protobuf/protobuf/third_party/benchmark` | submodule | `360e66c1c4777c99402cf8cd535aa510fee16573` | PRESENT | OWN | `LICENSE` `cfc7749b96f6…` |
| `3party/pugixml/pugixml` | submodule | `caade5a28aad86b92a4b5337a9dc70c4ba73c5eb` | PRESENT | OWN | `LICENSE.md` `0d0b3772af2f…` |
| `3party/utfcpp` | submodule | `6be08bbea14ffa0a5c594257fb6285a054395cd7` | PRESENT | OWN | `LICENSE` `c9bff7573892…` |
| `3party/utfcpp/extern/ftest` | submodule | `c4ad4af0946b73ce1a40cbc72205d15d196c7e06` | PRESENT | OWN | `LICENSE` `2c6a01b25167…` |
| `tools/kothic` | submodule | `3dd47d5941891ed2e0e554058edf214e9050e678` | PRESENT | OWN | `LICENSE.txt` `afd1514367ef…` |
| `tools/osmctools` | submodule | `45bb767d7939aaca39901d494c080fc4e7916ba7` | PRESENT | OWN | `LICENSE` `f228fa966732…` |
| `3party/GL` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `glcorearb.h` `8331fbf5dfbe…` |
| `3party/agg` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `agg_vcgen_smooth_poly1.h` `a583877a3e01…` |
| `3party/ankerl` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `stl.h` `954c0d08b6e1…` |
| `3party/bsdiff-courgette` | vendored | `vendored` | PRESENT | OWN | `LICENCE` `0af2a39b2cb3…`, `LICENSE` `29f948fde0da…` |
| `3party/freetype` | vendored | `vendored` | PRESENT | OWN | `LICENSE.TXT` `2e3bbb7d7c5c…`, `LICENSE` `c9bff7573892…` |
| `3party/harfbuzz` | vendored | `vendored` | PRESENT | OWN | `COPYING` `ba8f810f2455…`, `COPYING` `c2cfccb812fe…`, `COPYING` `9b35b8485f69…`, `COPYING` `f55f74bf740d…`, `COPYING` `d24039e4d22c…` |
| `3party/icu` | vendored | `vendored` | PRESENT | OWN | `LICENSE` `3ae033da0bfd…`, `LICENSE` `3ae033da0bfd…`, `COPYING` `4af93c12062c…`, `LICENSE` `4af93c12062c…` |
| `3party/imgui` | vendored | `vendored` | PRESENT | OWN | `LICENSE.txt` `0ebf8feb0615…`, `COPYING.txt` `da2fb2c80253…` |
| `3party/jansson` | vendored | `vendored` | PRESENT | OWN | `LICENSE` `150d90904cd8…` |
| `3party/kdtree++` | vendored | `vendored` | RESOLVED_REPO_LICENSE | RESOLVED_REPO_LICENSE | `region.hpp` `f30a6c539746…`, `Artistic-2.0.txt` `bae466ac8d71…` |
| `3party/libtess2` | vendored | `vendored` | PRESENT | OWN | `LICENSE.txt` `a098305be22e…` |
| `3party/minizip` | vendored | `vendored` | RESOLVED_REPO_LICENSE | RESOLVED_REPO_LICENSE | `MiniZip64_info.txt` `122719c32ef1…`, `Info-ZIP.txt` `fa0c6949eee1…` |
| `3party/monocypher` | vendored | `vendored` | PRESENT | OWN | `LICENCE.md` `5f8360e4c06d…` |
| `3party/open-location-code` | vendored | `vendored` | PRESENT | OWN | `LICENSE` `cfc7749b96f6…` |
| `3party/opening_hours` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `opening_hours.hpp` `ae35afb06ecc…` |
| `3party/protobuf` | vendored | `vendored` | PRESENT | OWN | `LICENSE` `af67c58de2e1…`, `LICENSE` `cfc7749b96f6…` |
| `3party/pugixml` | vendored | `vendored` | PRESENT | OWN | `LICENSE.md` `0d0b3772af2f…` |
| `3party/robust` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `predicates.c` `4aa3f8f52eb4…` |
| `3party/skarupke` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `__flat_hash_map.hpp` `ddb4c95467f2…` |
| `3party/stb_image` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `stb_image.h` `91f435e0fc6a…` |
| `3party/succinct` | vendored | `vendored` | PRESENT | OWN | `LICENSE` `567f39b9808a…` |
| `3party/vulkan_wrapper` | vendored | `vendored` | RESOLVED_SOURCE_HEADER | RESOLVED_SOURCE_HEADER | `vulkan_wrapper.h` `c8fb702c3b5f…` |

Artifact integrity hashes are retained in build.json/audit.json beside the candidate.
