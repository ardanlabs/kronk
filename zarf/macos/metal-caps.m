// =============================================================================
// metal-caps.m
//
// Print what Metal actually reports on the machine it runs on, including the
// three capability flags llama.cpp derives from it. Built for probing the
// ephemeral macOS runner VMs without a CI round trip, a kronk build, or a model
// download.
//
//   clang -fobjc-arc -framework Metal -framework Foundation -o metal-caps metal-caps.m
//   ./metal-caps
//
// Under the cua capability shim, pass its variables through:
//
//   DYLD_INSERT_LIBRARIES=./LumeMetalCapabilities-arm64.dylib \
//   LUME_METAL_APPLE_FAMILY_MAX=1009 ./metal-caps
//
// The gate arithmetic below mirrors ggml/src/ggml-metal/ggml-metal-device.m:
// 731-738. Keep them in sync; if llama.cpp changes a gate, this probe stops
// answering the question it exists to answer.
//
// A "yes" here is a REPORTED capability, not a working one. On a paravirtual
// GPU the two can differ, which is the whole reason this file exists.
// =============================================================================

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

int main(void) {
    @autoreleasepool {
        id<MTLDevice> dev = MTLCreateSystemDefaultDevice();
        if (dev == nil) {
            fprintf(stderr, "no Metal device\n");
            return 1;
        }

        printf("device   : %s\n", dev.name.UTF8String);
        printf("headless : %s\n", dev.isHeadless ? "yes" : "no");
        printf("unified  : %s\n", dev.hasUnifiedMemory ? "yes" : "no");
        printf("threadgroup memory : %lu bytes\n",
               (unsigned long)dev.maxThreadgroupMemoryLength);
        printf("recommended working set : %.2f MB\n",
               dev.recommendedMaxWorkingSetSize / 1024.0 / 1024.0);

        const struct { const char *name; MTLGPUFamily family; } families[] = {
            {"Apple1",  MTLGPUFamilyApple1},  {"Apple2",  MTLGPUFamilyApple2},
            {"Apple3",  MTLGPUFamilyApple3},  {"Apple4",  MTLGPUFamilyApple4},
            {"Apple5",  MTLGPUFamilyApple5},  {"Apple6",  MTLGPUFamilyApple6},
            {"Apple7",  MTLGPUFamilyApple7},  {"Apple8",  MTLGPUFamilyApple8},
            {"Apple9",  MTLGPUFamilyApple9},
            {"Common1", MTLGPUFamilyCommon1}, {"Common2", MTLGPUFamilyCommon2},
            {"Common3", MTLGPUFamilyCommon3},
            {"Metal3",  MTLGPUFamilyMetal3},
        };

        printf("\nsupportsFamily:\n");
        for (size_t i = 0; i < sizeof(families) / sizeof(families[0]); i++) {
            printf("  %-8s %s\n", families[i].name,
                   [dev supportsFamily:families[i].family] ? "yes" : "no");
        }

        // Metal3 is the honest probe: the cua shim deliberately never fakes it,
        // because MLX-LM uses that answer to select residency sets a
        // paravirtual device cannot create. It therefore stays "no" both on a
        // stock VM and on a shimmed one, which makes it the one capability that
        // still distinguishes a VM from real hardware.
        const BOOL apple6 = [dev supportsFamily:MTLGPUFamilyApple6];
        const BOOL apple7 = [dev supportsFamily:MTLGPUFamilyApple7];
        const BOOL metal3 = [dev supportsFamily:MTLGPUFamilyMetal3];

        printf("\nllama.cpp gates (ggml-metal-device.m:731-738):\n");
        printf("  has_simdgroup_reduction = %s   (Apple7 || Metal3)\n",
               (apple7 || metal3) ? "true" : "false");
        printf("  has_simdgroup_mm        = %s   (Apple7)\n",
               apple7 ? "true" : "false");
        printf("  has_bfloat              = %s   (Metal3 || Apple6)\n",
               (metal3 || apple6) ? "true" : "false");

        printf("\nreal hardware (Metal3) : %s\n", metal3 ? "yes" : "no (paravirtual GPU)");
    }

    return 0;
}
