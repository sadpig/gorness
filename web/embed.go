package web

import "embed"

// Assets 内嵌零构建链 Apple HIG 三栏工作台与实时 SVG 多智能体编排画布（index.html, styles.css, app.js）。
//
//go:embed index.html styles.css app.js
var Assets embed.FS
