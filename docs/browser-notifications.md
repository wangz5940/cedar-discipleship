# 主屏幕安装与系统通知

个人设置中的“安装与通知”负责当前设备；添加到主屏幕和系统通知授权是两个独立操作。

## 使用步骤

1. 从 HTTPS 地址登录，进入个人设置。
2. 点击“添加到主屏幕”。支持安装事件时显示浏览器原生安装窗口；否则按页面指引从浏览器菜单安装。
3. 点击“开启系统通知”，在原生授权弹窗选择允许。
4. 状态必须显示“后台推送已绑定”，才能确认服务器已绑定设备；“系统已授权，后台推送尚未绑定”表示仍需完成订阅。
5. 退出到桌面，由其他成员发送提醒，验证锁屏通知；点击后确认移除且不重复弹出。

## 浏览器适配边界

| 浏览器 | 主屏幕安装 | 系统通知 |
| --- | --- | --- |
| Chrome / Edge 安卓版 | 原生安装事件可用时直接调用，否则使用菜单 | 实际提供 Notification、Service Worker、PushManager 且订阅成功时使用 Web Push |
| Firefox 安卓版 | 使用浏览器菜单；不假定提供原生安装事件 | 按实际接口与订阅结果判断 |
| Samsung Internet | 原生安装事件或浏览器菜单 | 按实际接口与订阅结果判断 |
| 小米 / 华为 / UC 等浏览器 | 原生事件或菜单；桌面快捷方式不保证推送支持 | 不按品牌默认支持，缺少接口时显示具体限制 |
| Safari iOS / iPadOS | 分享菜单添加到主屏幕 | 从主屏幕打开后，由用户点击调用系统授权；保留既有推送流程 |

通知授权、配置读取、后台服务启动、设备订阅和服务器绑定分别报告失败阶段。设备订阅失败可能涉及浏览器推送服务或网络，不能仅凭授权成功断定后台推送可用。网站不集成厂商原生 App 推送 SDK，也不能绕过浏览器缺失的接口或受限网络。

清单沿用原应用 ID、启动地址与范围；安装服务不缓存账号页面或 API 响应。拒绝授权或绑定失败不会改动静音或历史学习记录。不同浏览器、设备分别授权和订阅。

## 验证范围

自动化覆盖原生安装事件、菜单回退、独立窗口识别、能力缺失、订阅失败和绑定重试；这些不是安卓手机真机结果。各浏览器实际安装、锁屏推送、通知权限和通知点击仍需按上述步骤在设备上验证。

参考：[Chrome 安装条件](https://developer.chrome.com/blog/update-install-criteria)、[Edge PWA 开发](https://learn.microsoft.com/en-us/microsoft-edge/progressive-web-apps-chromium/how-to/)、[Samsung 浏览器开发指南](https://developer.samsung.com/browser/android/web-developer-guide.html)、[Apple Web Push](https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/)。
