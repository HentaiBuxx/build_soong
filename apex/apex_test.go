// Copyright 2018 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apex

import (
	"android/soong/android"
	"android/soong/cc"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func testApex(t *testing.T, bp string) *android.TestContext {
	config, buildDir := setup(t)
	defer teardown(buildDir)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("apex", android.ModuleFactoryAdaptor(apexBundleFactory))
	ctx.RegisterModuleType("apex_key", android.ModuleFactoryAdaptor(apexKeyFactory))

	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("apex_deps", apexDepsMutator)
		ctx.BottomUp("apex", apexMutator)
	})

	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_library_shared", android.ModuleFactoryAdaptor(cc.LibrarySharedFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})

	ctx.Register()

	bp = bp + `
		toolchain_library {
			name: "libcompiler_rt-extras",
			src: "",
		}

		toolchain_library {
			name: "libatomic",
			src: "",
		}

		toolchain_library {
			name: "libgcc",
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-arm-android",
			src: "",
		}

		cc_object {
			name: "crtbegin_so",
			stl: "none",
		}

		cc_object {
			name: "crtend_so",
			stl: "none",
		}

	`

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp":                                []byte(bp),
		"testkey.avbpubkey":                         nil,
		"testkey.pem":                               nil,
		"build/target/product/security":             nil,
		"apex_manifest.json":                        nil,
		"system/sepolicy/apex/myapex-file_contexts": nil,
		"mylib.cpp":                                 nil,
	})
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx
}

func setup(t *testing.T) (config android.Config, buildDir string) {
	buildDir, err := ioutil.TempDir("", "soong_apex_test")
	if err != nil {
		t.Fatal(err)
	}

	config = android.TestArchConfig(buildDir, nil)

	return
}

func teardown(buildDir string) {
	os.RemoveAll(buildDir)
}

// ensure that 'result' contains 'expected'
func ensureContains(t *testing.T, result string, expected string) {
	if !strings.Contains(result, expected) {
		t.Errorf("%q is not found in %q", expected, result)
	}
}

// ensures that 'result' does not contain 'notExpected'
func ensureNotContains(t *testing.T, result string, notExpected string) {
	if strings.Contains(result, notExpected) {
		t.Errorf("%q is found in %q", notExpected, result)
	}
}

func ensureListContains(t *testing.T, result []string, expected string) {
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func ensureListNotContains(t *testing.T, result []string, notExpected string) {
	if android.InList(notExpected, result) {
		t.Errorf("%q is found in %v", notExpected, result)
	}
}

// Minimal test
func TestBasicApex(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_myapex")

	// Ensure that apex variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_myapex")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image/lib64/mylib.so")
	ensureContains(t, copyCmds, "image/lib64/mylib2.so")
}

func TestApexWithStubs(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib3"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2", "mylib3"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

		cc_library {
			name: "mylib3",
				srcs: ["mylib.cpp"],
				system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "11", "12"],
			},
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image/lib64/mylib2.so")

	// Ensure that direct stubs dep is included
	ensureContains(t, copyCmds, "image/lib64/mylib3.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_myapex").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stubs for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_3_myapex/mylib2.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_myapex/mylib2.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because mylib3 is in the same apex)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_myapex/mylib3.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_12_myapex/mylib3.so")
}

func TestApexWithSystemLibsStubs(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib_shared", "libdl", "libm"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
		}

		cc_library_shared {
			name: "mylib_shared",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
		}

		cc_library {
			name: "libc",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}

		cc_library {
			name: "libm",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}

		cc_library {
			name: "libdl",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["27", "28", "29"],
			},
		}
	`)

	apexRule := ctx.ModuleForTests("myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that mylib, libm, libdl are included.
	ensureContains(t, copyCmds, "image/lib64/mylib.so")
	ensureContains(t, copyCmds, "image/lib64/libm.so")
	ensureContains(t, copyCmds, "image/lib64/libdl.so")

	// Ensure that libc is not included (since it has stubs and not listed in native_shared_libs)
	ensureNotContains(t, copyCmds, "image/lib64/libc.so")

	mylibLdFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_shared_myapex").Rule("ld").Args["libFlags"]
	mylibCFlags := ctx.ModuleForTests("mylib", "android_arm64_armv8-a_static_myapex").Rule("cc").Args["cFlags"]
	mylibSharedCFlags := ctx.ModuleForTests("mylib_shared", "android_arm64_armv8-a_shared_myapex").Rule("cc").Args["cFlags"]

	// For dependency to libc
	// Ensure that mylib is linking with the latest version of stubs
	ensureContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_shared_29_myapex/libc.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_shared_myapex/libc.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBC_API__=29")
	ensureContains(t, mylibSharedCFlags, "__LIBC_API__=29")

	// For dependency to libm
	// Ensure that mylib is linking with the non-stub (impl) variant
	ensureContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_myapex/libm.so")
	// ... and not linking to the stub variant
	ensureNotContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_29_myapex/libm.so")
	// ... and is not compiling with the stub
	ensureNotContains(t, mylibCFlags, "__LIBM_API__=29")
	ensureNotContains(t, mylibSharedCFlags, "__LIBM_API__=29")

	// For dependency to libdl
	// Ensure that mylib is linking with the specified version of stubs
	ensureContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_27_myapex/libdl.so")
	// ... and not linking to the other versions of stubs
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_28_myapex/libdl.so")
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_29_myapex/libdl.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_myapex/libdl.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBDL_API__=27")
	ensureContains(t, mylibSharedCFlags, "__LIBDL_API__=27")
}