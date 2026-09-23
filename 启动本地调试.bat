chcp 65001 >nul
@echo off
title Cedar Discipleship - 本地开发调试
cls
echo ======================================================
echo         Cedar Discipleship - 本地开发调试
echo ======================================================
echo.
echo   本地前端地址: http://localhost:5173
echo   后端接口代理: http://mouss.synology.me:5114
echo   说明: cedar-discipleship 官方前端工程
echo   支持热重载: 修改文件后浏览器立即自动刷新生效
echo.
echo 正在启动 Vite 开发调试服务器，请稍候...
echo ======================================================
echo.

cd /d "%~dp0frontend"

if not exist node_modules (
    echo [提示] 首次运行，正在安装前端依赖...
    call npm install
)

call npm run dev
pause