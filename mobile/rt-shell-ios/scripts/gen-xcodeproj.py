#!/usr/bin/env python3
"""Generate a minimal Xcode project for Baize MOBILE-RT iOS shell."""
from __future__ import annotations

import pathlib
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[1]
PROJ = ROOT / "BaizeMobileRT.xcodeproj"
PBX = PROJ / "project.pbxproj"


def uid() -> str:
    return uuid.uuid4().hex[:24].upper()


IDS = {
    "project": uid(),
    "target": uid(),
    "sources": uid(),
    "frameworks": uid(),
    "resources": uid(),
    "product": uid(),
    "group_root": uid(),
    "group_app": uid(),
    "group_products": uid(),
    "group_fw": uid(),
    "cfg_proj_debug": uid(),
    "cfg_proj_release": uid(),
    "cfg_tgt_debug": uid(),
    "cfg_tgt_release": uid(),
    "cfg_list_proj": uid(),
    "cfg_list_tgt": uid(),
    "file_app": uid(),
    "file_content": uid(),
    "file_cred": uid(),
    "file_info": uid(),
    "file_bridge": uid(),
    "file_xcfw": uid(),
    "build_app": uid(),
    "build_content": uid(),
    "build_cred": uid(),
    "build_info": uid(),
    "build_xcfw": uid(),
}

pbx = f"""// !$*UTF8*$!
{{
	archiveVersion = 1;
	classes = {{
	}};
	objectVersion = 56;
	objects = {{

/* Begin PBXBuildFile section */
		{IDS["build_app"]} /* BaizeMobileRTApp.swift in Sources */ = {{isa = PBXBuildFile; fileRef = {IDS["file_app"]} /* BaizeMobileRTApp.swift */; }};
		{IDS["build_content"]} /* ContentView.swift in Sources */ = {{isa = PBXBuildFile; fileRef = {IDS["file_content"]} /* ContentView.swift */; }};
		{IDS["build_cred"]} /* CredentialStore.swift in Sources */ = {{isa = PBXBuildFile; fileRef = {IDS["file_cred"]} /* CredentialStore.swift */; }};
		{IDS["build_info"]} /* Info.plist in Resources */ = {{isa = PBXBuildFile; fileRef = {IDS["file_info"]} /* Info.plist */; }};
		{IDS["build_xcfw"]} /* Mobileagent.xcframework in Frameworks */ = {{isa = PBXBuildFile; fileRef = {IDS["file_xcfw"]} /* Mobileagent.xcframework */; }};
/* End PBXBuildFile section */

/* Begin PBXFileReference section */
		{IDS["product"]} /* BaizeMobileRT.app */ = {{isa = PBXFileReference; explicitFileType = wrapper.application; includeInIndex = 0; path = BaizeMobileRT.app; sourceTree = BUILT_PRODUCTS_DIR; }};
		{IDS["file_app"]} /* BaizeMobileRTApp.swift */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = BaizeMobileRTApp.swift; sourceTree = "<group>"; }};
		{IDS["file_content"]} /* ContentView.swift */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = ContentView.swift; sourceTree = "<group>"; }};
		{IDS["file_cred"]} /* CredentialStore.swift */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = CredentialStore.swift; sourceTree = "<group>"; }};
		{IDS["file_info"]} /* Info.plist */ = {{isa = PBXFileReference; lastKnownFileType = text.plist.xml; path = Info.plist; sourceTree = "<group>"; }};
		{IDS["file_bridge"]} /* Bridging-Header.h */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.c.h; path = "Bridging-Header.h"; sourceTree = "<group>"; }};
		{IDS["file_xcfw"]} /* Mobileagent.xcframework */ = {{isa = PBXFileReference; lastKnownFileType = wrapper.xcframework; path = Mobileagent.xcframework; sourceTree = "<group>"; }};
/* End PBXFileReference section */

/* Begin PBXFrameworksBuildPhase section */
		{IDS["frameworks"]} /* Frameworks */ = {{
			isa = PBXFrameworksBuildPhase;
			buildActionMask = 2147483647;
			files = (
				{IDS["build_xcfw"]} /* Mobileagent.xcframework in Frameworks */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXFrameworksBuildPhase section */

/* Begin PBXGroup section */
		{IDS["group_root"]} = {{
			isa = PBXGroup;
			children = (
				{IDS["group_app"]} /* BaizeMobileRT */,
				{IDS["group_fw"]} /* Frameworks */,
				{IDS["group_products"]} /* Products */,
			);
			sourceTree = "<group>";
		}};
		{IDS["group_app"]} /* BaizeMobileRT */ = {{
			isa = PBXGroup;
			children = (
				{IDS["file_app"]} /* BaizeMobileRTApp.swift */,
				{IDS["file_content"]} /* ContentView.swift */,
				{IDS["file_cred"]} /* CredentialStore.swift */,
				{IDS["file_bridge"]} /* Bridging-Header.h */,
				{IDS["file_info"]} /* Info.plist */,
			);
			path = BaizeMobileRT;
			sourceTree = "<group>";
		}};
		{IDS["group_fw"]} /* Frameworks */ = {{
			isa = PBXGroup;
			children = (
				{IDS["file_xcfw"]} /* Mobileagent.xcframework */,
			);
			path = Frameworks;
			sourceTree = "<group>";
		}};
		{IDS["group_products"]} /* Products */ = {{
			isa = PBXGroup;
			children = (
				{IDS["product"]} /* BaizeMobileRT.app */,
			);
			name = Products;
			sourceTree = "<group>";
		}};
/* End PBXGroup section */

/* Begin PBXNativeTarget section */
		{IDS["target"]} /* BaizeMobileRT */ = {{
			isa = PBXNativeTarget;
			buildConfigurationList = {IDS["cfg_list_tgt"]} /* Build configuration list for PBXNativeTarget "BaizeMobileRT" */;
			buildPhases = (
				{IDS["sources"]} /* Sources */,
				{IDS["frameworks"]} /* Frameworks */,
				{IDS["resources"]} /* Resources */,
			);
			buildRules = (
			);
			dependencies = (
			);
			name = BaizeMobileRT;
			productName = BaizeMobileRT;
			productReference = {IDS["product"]} /* BaizeMobileRT.app */;
			productType = "com.apple.product-type.application";
		}};
/* End PBXNativeTarget section */

/* Begin PBXProject section */
		{IDS["project"]} /* Project object */ = {{
			isa = PBXProject;
			attributes = {{
				BuildIndependentTargetsInParallel = 1;
				LastSwiftUpdateCheck = 1500;
				LastUpgradeCheck = 1500;
			}};
			buildConfigurationList = {IDS["cfg_list_proj"]} /* Build configuration list for PBXProject "BaizeMobileRT" */;
			compatibilityVersion = "Xcode 14.0";
			developmentRegion = en;
			hasScannedForEncodings = 0;
			knownRegions = (
				en,
				Base,
			);
			mainGroup = {IDS["group_root"]};
			productRefGroup = {IDS["group_products"]} /* Products */;
			projectDirPath = "";
			projectRoot = "";
			targets = (
				{IDS["target"]} /* BaizeMobileRT */,
			);
		}};
/* End PBXProject section */

/* Begin PBXResourcesBuildPhase section */
		{IDS["resources"]} /* Resources */ = {{
			isa = PBXResourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXResourcesBuildPhase section */

/* Begin PBXSourcesBuildPhase section */
		{IDS["sources"]} /* Sources */ = {{
			isa = PBXSourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
				{IDS["build_app"]} /* BaizeMobileRTApp.swift in Sources */,
				{IDS["build_content"]} /* ContentView.swift in Sources */,
				{IDS["build_cred"]} /* CredentialStore.swift in Sources */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		}};
/* End PBXSourcesBuildPhase section */

/* Begin XCBuildConfiguration section */
		{IDS["cfg_proj_debug"]} /* Debug */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{
				ALWAYS_SEARCH_USER_PATHS = NO;
				CLANG_ENABLE_MODULES = YES;
				CLANG_ENABLE_OBJC_ARC = YES;
				COPY_PHASE_STRIP = NO;
				DEBUG_INFORMATION_FORMAT = dwarf;
				ENABLE_TESTABILITY = YES;
				GCC_DYNAMIC_NO_PIC = NO;
				IPHONEOS_DEPLOYMENT_TARGET = 16.0;
				ONLY_ACTIVE_ARCH = YES;
				SDKROOT = iphoneos;
				SWIFT_ACTIVE_COMPILATION_CONDITIONS = DEBUG;
				SWIFT_OPTIMIZATION_LEVEL = "-Onone";
			}};
			name = Debug;
		}};
		{IDS["cfg_proj_release"]} /* Release */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{
				ALWAYS_SEARCH_USER_PATHS = NO;
				CLANG_ENABLE_MODULES = YES;
				CLANG_ENABLE_OBJC_ARC = YES;
				COPY_PHASE_STRIP = NO;
				DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym";
				IPHONEOS_DEPLOYMENT_TARGET = 16.0;
				SDKROOT = iphoneos;
				SWIFT_COMPILATION_MODE = wholemodule;
			}};
			name = Release;
		}};
		{IDS["cfg_tgt_debug"]} /* Debug */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{
				CODE_SIGN_STYLE = Automatic;
				CURRENT_PROJECT_VERSION = 1;
				DEVELOPMENT_TEAM = "";
				ENABLE_PREVIEWS = YES;
				FRAMEWORK_SEARCH_PATHS = (
					"$(PROJECT_DIR)/Frameworks",
				);
				GENERATE_INFOPLIST_FILE = NO;
				INFOPLIST_FILE = BaizeMobileRT/Info.plist;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
				);
				MARKETING_VERSION = 0.1.0;
				PRODUCT_BUNDLE_IDENTIFIER = com.rebornace.baize.mobilert;
				PRODUCT_NAME = "$(TARGET_NAME)";
				SUPPORTED_PLATFORMS = "iphoneos iphonesimulator";
				SUPPORTS_MACCATALYST = NO;
				SWIFT_EMIT_LOC_STRINGS = YES;
				SWIFT_OBJC_BRIDGING_HEADER = "BaizeMobileRT/Bridging-Header.h";
				SWIFT_VERSION = 5.0;
				TARGETED_DEVICE_FAMILY = "1,2";
			}};
			name = Debug;
		}};
		{IDS["cfg_tgt_release"]} /* Release */ = {{
			isa = XCBuildConfiguration;
			buildSettings = {{
				CODE_SIGN_STYLE = Automatic;
				CURRENT_PROJECT_VERSION = 1;
				DEVELOPMENT_TEAM = "";
				ENABLE_PREVIEWS = YES;
				FRAMEWORK_SEARCH_PATHS = (
					"$(PROJECT_DIR)/Frameworks",
				);
				GENERATE_INFOPLIST_FILE = NO;
				INFOPLIST_FILE = BaizeMobileRT/Info.plist;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
				);
				MARKETING_VERSION = 0.1.0;
				PRODUCT_BUNDLE_IDENTIFIER = com.rebornace.baize.mobilert;
				PRODUCT_NAME = "$(TARGET_NAME)";
				SUPPORTED_PLATFORMS = "iphoneos iphonesimulator";
				SUPPORTS_MACCATALYST = NO;
				SWIFT_EMIT_LOC_STRINGS = YES;
				SWIFT_OBJC_BRIDGING_HEADER = "BaizeMobileRT/Bridging-Header.h";
				SWIFT_VERSION = 5.0;
				TARGETED_DEVICE_FAMILY = "1,2";
			}};
			name = Release;
		}};
/* End XCBuildConfiguration section */

/* Begin XCConfigurationList section */
		{IDS["cfg_list_proj"]} /* Build configuration list for PBXProject "BaizeMobileRT" */ = {{
			isa = XCConfigurationList;
			buildConfigurations = (
				{IDS["cfg_proj_debug"]} /* Debug */,
				{IDS["cfg_proj_release"]} /* Release */,
			);
			defaultConfigurationIsVisible = 0;
			defaultConfigurationName = Release;
		}};
		{IDS["cfg_list_tgt"]} /* Build configuration list for PBXNativeTarget "BaizeMobileRT" */ = {{
			isa = XCConfigurationList;
			buildConfigurations = (
				{IDS["cfg_tgt_debug"]} /* Debug */,
				{IDS["cfg_tgt_release"]} /* Release */,
			);
			defaultConfigurationIsVisible = 0;
			defaultConfigurationName = Release;
		}};
/* End XCConfigurationList section */
	}};
	rootObject = {IDS["project"]} /* Project object */;
}}
"""

PROJ.mkdir(parents=True, exist_ok=True)
PBX.write_text(pbx)
print(f"wrote {PBX}")
