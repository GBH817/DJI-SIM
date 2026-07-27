@echo off
chcp 65001 >nul
title 低空飞行数据生成器 - 一键启动

echo ========================================
echo   低空飞行数据生成器 - 一键启动
echo ========================================
echo.

:: 获取脚本所在目录
set "PROJECT_DIR=%~dp0"

:: ========== 检查 Go ==========
echo [1/4] 检查 Go 环境...

:: 自动查找 Go 安装位置
set "GO_PATH="
for /d %%d in ("%USERPROFILE%\sdk\go*") do (
    if exist "%%d\bin\go.exe" (
        set "GO_PATH=%%d\bin"
        set "PATH=%%d\bin;%PATH%"
    )
)
if not defined GO_PATH (
    if exist "C:\Program Files\Go\bin\go.exe" (
        set "GO_PATH=C:\Program Files\Go\bin"
        set "PATH=C:\Program Files\Go\bin;%PATH%"
    )
)

where go >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未找到 Go，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)
for /f "tokens=3" %%i in ('go version') do echo        Go 版本: %%i

:: ========== 检查 Node.js ==========
echo [2/4] 检查 Node.js 环境...
where node >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未找到 Node.js，请先安装 Node.js: https://nodejs.org/
    pause
    exit /b 1
)
for /f "tokens=1,2" %%i in ('node --version') do echo        Node 版本: %%i

:: ========== 后端依赖 ==========
echo [3/4] 安装后端依赖...
cd /d "%PROJECT_DIR%backend"
go mod tidy 2>&1
if %errorlevel% neq 0 (
    echo [警告] go mod tidy 有警告，继续启动...
)

:: 编译后端（验证代码无语法错误）
echo        编译检查...
go build -o server.exe ./cmd/server/ 2>&1
if %errorlevel% neq 0 (
    echo [错误] 后端编译失败！请检查错误信息。
    pause
    exit /b 1
)
echo        后端编译成功

:: ========== 前端依赖 ==========
cd /d "%PROJECT_DIR%frontend"
if not exist "node_modules" (
    echo        安装前端依赖...
    call npm install 2>&1
    if %errorlevel% neq 0 (
        echo [错误] npm install 失败！
        pause
        exit /b 1
    )
) else (
    echo        前端依赖已存在，跳过安装
)

:: ========== 启动服务 ==========
echo [4/4] 启动服务...
echo.
echo ========================================
echo   启动后端服务 (端口 8080)
echo   启动前端服务 (端口 5173)
echo ========================================
echo.
echo   打开浏览器访问:
echo   - 正常模式: http://localhost:5173
echo   - Debug模式: http://localhost:5173/?debug=1
echo.
echo   按任意键启动，或关闭此窗口停止所有服务
pause >nul

:: 启动后端（新窗口）
cd /d "%PROJECT_DIR%backend"
start "后端服务 - drone-sim-backend" cmd /c "go run ./cmd/server/ 2>&1 && pause"

:: 等待后端启动
echo        等待后端启动...
timeout /t 2 /nobreak >nul

:: 启动前端（新窗口）
cd /d "%PROJECT_DIR%frontend"
start "前端服务 - drone-sim-frontend" cmd /c "npm run dev 2>&1 && pause"

echo.
echo ========================================
echo   启动完成！
echo ========================================
echo.
echo   后端: http://localhost:8080
echo   前端: http://localhost:5173
echo   Debug: http://localhost:5173/?debug=1
echo.
echo   关闭新打开的两个命令行窗口即可停止服务
echo ========================================

:: 保持主窗口打开
pause
