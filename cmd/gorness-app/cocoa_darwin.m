//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdlib.h>
#include <string.h>

extern int goDecideNavigation(char *targetURL);
extern char *goOnFolderChosen(char *folderPath);
extern int goPollAttentionCount(void);
extern void goOpenVaultInFinder(void);

@interface GornessAppDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKUIDelegate>
@property (strong, nonatomic) NSWindow *mainWindow;
@property (strong, nonatomic) WKWebView *webView;
@property (strong, nonatomic) NSStatusItem *statusItem;
@property (strong, nonatomic) NSMenuItem *attentionMenuItem;
@property (strong, nonatomic) NSTimer *attentionPollTimer;
@property (assign, nonatomic) BOOL attentionPollInFlight;
@property (strong, nonatomic) NSMutableArray<NSWindow *> *previewWindows;
@property (copy, nonatomic) NSString *baseURLString;
@end

@implementation GornessAppDelegate

- (instancetype)initWithBaseURL:(NSString *)urlStr {
	self = [super init];
	if (self) {
		_baseURLString = [urlStr copy];
		_previewWindows = [NSMutableArray array];
		_attentionPollInFlight = NO;
	}
	return self;
}

- (void)setupMenus {
	NSMenu *menubar = [[NSMenu alloc] init];

	// App Menu
	NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
	[menubar addItem:appMenuItem];
	NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"Gorness"];
	[appMenu addItemWithTitle:@"关于 Gorness" action:@selector(orderFrontStandardAboutPanel:) keyEquivalent:@""];
	[appMenu addItem:[NSMenuItem separatorItem]];
	NSMenuItem *settingsItem = [[NSMenuItem alloc] initWithTitle:@"设置与角色模型矩阵…" action:@selector(openSettings:) keyEquivalent:@","];
	[settingsItem setTarget:self];
	[appMenu addItem:settingsItem];
	[appMenu addItem:[NSMenuItem separatorItem]];
	NSMenuItem *openProj = [[NSMenuItem alloc] initWithTitle:@"添加项目文件夹…" action:@selector(addProjectFolder:) keyEquivalent:@"o"];
	[openProj setTarget:self];
	[appMenu addItem:openProj];
	NSMenuItem *openVault = [[NSMenuItem alloc] initWithTitle:@"在 Finder 中显示 ~/.gorness" action:@selector(revealVault:) keyEquivalent:@"G"];
	[openVault setKeyEquivalentModifierMask:(NSEventModifierFlagCommand | NSEventModifierFlagShift)];
	[openVault setTarget:self];
	[appMenu addItem:openVault];
	[appMenu addItem:[NSMenuItem separatorItem]];
	[appMenu addItemWithTitle:@"隐藏 Gorness" action:@selector(hide:) keyEquivalent:@"h"];
	NSMenuItem *hideOthers = [[NSMenuItem alloc] initWithTitle:@"隐藏其他" action:@selector(hideOtherApplications:) keyEquivalent:@"h"];
	[hideOthers setKeyEquivalentModifierMask:(NSEventModifierFlagCommand | NSEventModifierFlagOption)];
	[appMenu addItem:hideOthers];
	[appMenu addItemWithTitle:@"全部显示" action:@selector(unhideAllApplications:) keyEquivalent:@""];
	[appMenu addItem:[NSMenuItem separatorItem]];
	[appMenu addItemWithTitle:@"退出 Gorness" action:@selector(terminate:) keyEquivalent:@"q"];
	[appMenuItem setSubmenu:appMenu];

	// Edit Menu (essential for Cmd+C/V/X/A/Z in WKWebView inputs)
	NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
	[menubar addItem:editMenuItem];
	NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"编辑"];
	[editMenu addItemWithTitle:@"撤销" action:NSSelectorFromString(@"undo:") keyEquivalent:@"z"];
	NSMenuItem *redoItem = [[NSMenuItem alloc] initWithTitle:@"重做" action:NSSelectorFromString(@"redo:") keyEquivalent:@"Z"];
	[redoItem setKeyEquivalentModifierMask:(NSEventModifierFlagCommand | NSEventModifierFlagShift)];
	[editMenu addItem:redoItem];
	[editMenu addItem:[NSMenuItem separatorItem]];
	[editMenu addItemWithTitle:@"剪切" action:@selector(cut:) keyEquivalent:@"x"];
	[editMenu addItemWithTitle:@"拷贝" action:@selector(copy:) keyEquivalent:@"c"];
	[editMenu addItemWithTitle:@"粘贴" action:@selector(paste:) keyEquivalent:@"v"];
	[editMenu addItemWithTitle:@"全选" action:@selector(selectAll:) keyEquivalent:@"a"];
	[editMenuItem setSubmenu:editMenu];

	// View Menu
	NSMenuItem *viewMenuItem = [[NSMenuItem alloc] init];
	[menubar addItem:viewMenuItem];
	NSMenu *viewMenu = [[NSMenu alloc] initWithTitle:@"视图"];
	NSMenuItem *reloadItem = [[NSMenuItem alloc] initWithTitle:@"重新加载工作台" action:@selector(reloadWorkbench:) keyEquivalent:@"r"];
	[reloadItem setTarget:self];
	[viewMenu addItem:reloadItem];
	[viewMenu addItem:[NSMenuItem separatorItem]];
	NSMenuItem *zoomActual = [[NSMenuItem alloc] initWithTitle:@"实际大小" action:@selector(resetZoom:) keyEquivalent:@"0"];
	[zoomActual setTarget:self];
	[viewMenu addItem:zoomActual];
	NSMenuItem *zoomIn = [[NSMenuItem alloc] initWithTitle:@"放大" action:@selector(zoomIn:) keyEquivalent:@"="];
	[zoomIn setTarget:self];
	[viewMenu addItem:zoomIn];
	NSMenuItem *zoomOut = [[NSMenuItem alloc] initWithTitle:@"缩小" action:@selector(zoomOut:) keyEquivalent:@"-"];
	[zoomOut setTarget:self];
	[viewMenu addItem:zoomOut];
	[viewMenuItem setSubmenu:viewMenu];

	// Window Menu
	NSMenuItem *winMenuItem = [[NSMenuItem alloc] init];
	[menubar addItem:winMenuItem];
	NSMenu *winMenu = [[NSMenu alloc] initWithTitle:@"窗口"];
	[winMenu addItemWithTitle:@"最小化" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
	[winMenu addItemWithTitle:@"关闭窗口" action:@selector(performClose:) keyEquivalent:@"w"];
	[winMenuItem setSubmenu:winMenu];
	[NSApp setWindowsMenu:winMenu];

	[NSApp setMainMenu:menubar];
}

- (void)setupStatusBarItem {
	self.statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
	self.statusItem.button.title = @"G·";
	self.statusItem.button.font = [NSFont monospacedSystemFontOfSize:12.5 weight:NSFontWeightBold];

	NSMenu *menu = [[NSMenu alloc] init];
	self.attentionMenuItem = [[NSMenuItem alloc] initWithTitle:@"全部就绪 (0 待处理)" action:@selector(showMainWindow:) keyEquivalent:@""];
	[self.attentionMenuItem setTarget:self];
	[menu addItem:self.attentionMenuItem];
	[menu addItem:[NSMenuItem separatorItem]];

	NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"打开 Gorness 工作台" action:@selector(showMainWindow:) keyEquivalent:@""];
	[showItem setTarget:self];
	[menu addItem:showItem];

	NSMenuItem *statusSettingsItem = [[NSMenuItem alloc] initWithTitle:@"设置与角色模型矩阵…" action:@selector(openSettings:) keyEquivalent:@""];
	[statusSettingsItem setTarget:self];
	[menu addItem:statusSettingsItem];

	NSMenuItem *addItem = [[NSMenuItem alloc] initWithTitle:@"添加项目文件夹…" action:@selector(addProjectFolder:) keyEquivalent:@""];
	[addItem setTarget:self];
	[menu addItem:addItem];

	NSMenuItem *vaultItem = [[NSMenuItem alloc] initWithTitle:@"打开 ~/.gorness 配置目录" action:@selector(revealVault:) keyEquivalent:@""];
	[vaultItem setTarget:self];
	[menu addItem:vaultItem];

	[menu addItem:[NSMenuItem separatorItem]];
	[menu addItemWithTitle:@"退出 Gorness" action:@selector(terminate:) keyEquivalent:@""];
	self.statusItem.menu = menu;
}

- (void)updateAttentionBadgeWithCount:(int)count {
	if (self.statusItem == nil || self.statusItem.button == nil) {
		return;
	}
	if (count > 0) {
		self.statusItem.button.title = [NSString stringWithFormat:@"G·%d", count];
		if (self.attentionMenuItem != nil) {
			self.attentionMenuItem.title = [NSString stringWithFormat:@"待处理审批: %d 项", count];
		}
	} else {
		self.statusItem.button.title = @"G·";
		if (self.attentionMenuItem != nil) {
			self.attentionMenuItem.title = @"全部就绪 (0 待处理)";
		}
	}
}

- (void)pollAttentionBadge:(NSTimer *)timer {
	if (self.attentionPollInFlight) {
		return;
	}
	self.attentionPollInFlight = YES;
	__weak typeof(self) weakSelf = self;
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
		int count = goPollAttentionCount();
		dispatch_async(dispatch_get_main_queue(), ^{
			__strong typeof(weakSelf) strongSelf = weakSelf;
			if (strongSelf == nil) {
				return;
			}
			strongSelf.attentionPollInFlight = NO;
			[strongSelf updateAttentionBadgeWithCount:count];
		});
	});
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
	[self setupMenus];
	[self setupStatusBarItem];

	NSRect frame = NSMakeRect(0, 0, 1360, 860);
	NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
	self.mainWindow = [[NSWindow alloc] initWithContentRect:frame styleMask:style backing:NSBackingStoreBuffered defer:NO];
	[self.mainWindow setTitle:@"Gorness"];
	[self.mainWindow setMinSize:NSMakeSize(1080, 680)];
	[self.mainWindow setReleasedWhenClosed:NO];
	[self.mainWindow center];
	[self.mainWindow setFrameAutosaveName:@"GornessMainWindow"];

	WKWebViewConfiguration *cfg = [[WKWebViewConfiguration alloc] init];
	self.webView = [[WKWebView alloc] initWithFrame:[[self.mainWindow contentView] bounds] configuration:cfg];
	[self.webView setAutoresizingMask:(NSViewWidthSizable | NSViewHeightSizable)];
	[self.webView setNavigationDelegate:self];
	[self.webView setUIDelegate:self];

	[[self.mainWindow contentView] addSubview:self.webView];
	NSURL *url = [NSURL URLWithString:[self.baseURLString stringByAppendingString:@"/"]];
	[self.webView loadRequest:[NSURLRequest requestWithURL:url]];

	[self.mainWindow makeKeyAndOrderFront:nil];
	[NSApp activateIgnoringOtherApps:YES];

	[self pollAttentionBadge:nil];
	self.attentionPollTimer = [NSTimer scheduledTimerWithTimeInterval:2.0
	                                                           target:self
	                                                         selector:@selector(pollAttentionBadge:)
	                                                         userInfo:nil
	                                                          repeats:YES];
}

- (void)applicationWillTerminate:(NSNotification *)notification {
	if (self.attentionPollTimer != nil) {
		[self.attentionPollTimer invalidate];
		self.attentionPollTimer = nil;
	}
	for (NSWindow *win in [self.previewWindows copy]) {
		[win setDelegate:nil];
	}
	[self.previewWindows removeAllObjects];
}

- (void)windowWillClose:(NSNotification *)notification {
	NSWindow *closingWindow = [notification object];
	if (closingWindow != nil && closingWindow != self.mainWindow) {
		[closingWindow setDelegate:nil];
		[self.previewWindows removeObjectIdenticalTo:closingWindow];
	}
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
	if (!flag && self.mainWindow != nil) {
		[self.mainWindow makeKeyAndOrderFront:nil];
	}
	[NSApp activateIgnoringOtherApps:YES];
	return YES;
}

- (void)showMainWindow:(id)sender {
	if (self.mainWindow != nil) {
		[self.mainWindow makeKeyAndOrderFront:nil];
	}
	[NSApp activateIgnoringOtherApps:YES];
}

- (void)openSettings:(id)sender {
	[self showMainWindow:sender];
	if (self.webView != nil) {
		[self.webView evaluateJavaScript:@"if (window.openGornessSettingsModal) window.openGornessSettingsModal();" completionHandler:nil];
	}
}

- (void)reloadWorkbench:(id)sender {
	[self.webView reload];
}

- (void)resetZoom:(id)sender {
	if (@available(macOS 11.0, *)) {
		self.webView.pageZoom = 1.0;
	}
}

- (void)zoomIn:(id)sender {
	if (@available(macOS 11.0, *)) {
		self.webView.pageZoom = MIN(2.5, self.webView.pageZoom + 0.1);
	}
}

- (void)zoomOut:(id)sender {
	if (@available(macOS 11.0, *)) {
		self.webView.pageZoom = MAX(0.6, self.webView.pageZoom - 0.1);
	}
}

- (void)revealVault:(id)sender {
	goOpenVaultInFinder();
}

- (void)addProjectFolder:(id)sender {
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	[panel setCanChooseFiles:NO];
	[panel setCanChooseDirectories:YES];
	[panel setAllowsMultipleSelection:NO];
	[panel setPrompt:@"挂载项目"];
	[panel setMessage:@"选择要挂载到 Gorness 的本地代码仓库文件夹"];
	if ([panel runModal] == NSModalResponseOK) {
		NSURL *chosen = [[panel URLs] firstObject];
		if (chosen != nil && [chosen path] != nil) {
			const char *cstr = [[chosen path] UTF8String];
			char *rawWorkspaceJSON = goOnFolderChosen((char *)cstr);
			if (rawWorkspaceJSON == NULL) {
				return;
			}
			NSString *wsJSON = [NSString stringWithUTF8String:rawWorkspaceJSON];
			free(rawWorkspaceJSON);

			if (wsJSON.length == 0 || self.webView == nil) {
				[self.webView reload];
				return;
			}

			NSError *serErr = nil;
			NSData *arrData = [NSJSONSerialization dataWithJSONObject:@[wsJSON] options:0 error:&serErr];
			NSString *arrLiteral = (arrData != nil && serErr == nil)
				? [[NSString alloc] initWithData:arrData encoding:NSUTF8StringEncoding]
				: nil;
			if (arrLiteral.length == 0) {
				[self.webView reload];
				return;
			}

			NSString *js = [NSString stringWithFormat:
				@"(function(){ if (typeof window.onNativeWorkspaceRegistered === 'function') { void window.onNativeWorkspaceRegistered((%@)[0]); return true; } return false; })()",
				arrLiteral];
			__weak typeof(self) weakSelf = self;
			[self.webView evaluateJavaScript:js completionHandler:^(id result, NSError *error) {
				if (error != nil || ![result respondsToSelector:@selector(boolValue)] || ![result boolValue]) {
					[weakSelf.webView reload];
				}
			}];
			[self pollAttentionBadge:nil];
		}
	}
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	NSURL *target = navigationAction.request.URL;
	if (target == nil) {
		decisionHandler(WKNavigationActionPolicyCancel);
		return;
	}
	NSString *absStr = [target absoluteString];
	if ([absStr hasPrefix:@"about:"] || [absStr hasPrefix:@"blob:"]) {
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	int verdict = goDecideNavigation((char *)[absStr UTF8String]);
	if (verdict == 1) {
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	if (verdict == 2) {
		[[NSWorkspace sharedWorkspace] openURL:target];
	}
	decisionHandler(WKNavigationActionPolicyCancel);
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
	NSURL *target = navigationAction.request.URL;
	if (target == nil) {
		return nil;
	}
	int verdict = goDecideNavigation((char *)[[target absoluteString] UTF8String]);
	if (verdict == 2) {
		[[NSWorkspace sharedWorkspace] openURL:target];
		return nil;
	}
	if (verdict != 1) {
		return nil;
	}

	NSRect pFrame = NSMakeRect(0, 0, 980, 700);
	NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
	NSWindow *childWin = [[NSWindow alloc] initWithContentRect:pFrame styleMask:style backing:NSBackingStoreBuffered defer:NO];
	[childWin setTitle:@"Gorness 隔离沙箱预览"];
	[childWin setReleasedWhenClosed:NO];
	[childWin setDelegate:self];
	[childWin center];
	WKWebView *childWeb = [[WKWebView alloc] initWithFrame:[[childWin contentView] bounds] configuration:configuration];
	[childWeb setAutoresizingMask:(NSViewWidthSizable | NSViewHeightSizable)];
	[childWeb setNavigationDelegate:self];
	[[childWin contentView] addSubview:childWeb];
	[self.previewWindows addObject:childWin];
	[childWin makeKeyAndOrderFront:nil];
	return childWeb;
}

@end

void runCocoaDesktop(const char *baseURL, const char *errMsg) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		if (errMsg != NULL && strlen(errMsg) > 0) {
			NSAlert *alert = [[NSAlert alloc] init];
			[alert setAlertStyle:NSAlertStyleCritical];
			[alert setMessageText:@"Gorness 引擎启动失败"];
			[alert setInformativeText:[NSString stringWithUTF8String:errMsg]];
			[alert runModal];
			return;
		}
		GornessAppDelegate *del = [[GornessAppDelegate alloc] initWithBaseURL:[NSString stringWithUTF8String:baseURL]];
		[app setDelegate:del];
		[app run];
	}
}
