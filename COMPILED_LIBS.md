# sing-box 编译库总结

## 版本信息
- **sing-box 版本**: v1.12.12 (稳定版)
- **编译类型**: 生产版本（Release，无 debug 标志）
- **编译日期**: 2025-11-18

---

## 📱 iOS 库

### 位置
```
sing-box/libbox_output/Libbox.xcframework
```

### 架构支持
- ✅ **iOS arm64** (真机 - iPhone/iPad)
- ✅ **iOS Simulator arm64** (Apple Silicon Mac 模拟器)
- ✅ **iOS Simulator x86_64** (Intel Mac 模拟器)

### 文件大小
~96 MB

### 使用
```bash
cp -R sing-box/libbox_output/Libbox.xcframework \
      SingBoxVPN-iOS/Frameworks/
```

---

## 💻 macOS 库

### 位置
```
sing-box/libbox_output_macos/Libbox.xcframework
```

### 架构支持
- ✅ **macOS arm64** (Apple Silicon - M1/M2/M3/M4 芯片)
- ✅ **macOS x86_64** (Intel 芯片)

### Universal Binary
单个文件包含所有架构，系统自动选择正确的架构运行。

### 文件大小
~82 MB

### 使用
```bash
cp -R sing-box/libbox_output_macos/Libbox.xcframework \
      macOSProject/Frameworks/
```

---

## 🔧 编译脚本

### iOS
```bash
cd sing-box
./build_all_ios.sh
```

### macOS
```bash
cd sing-box
./build_all_macos.sh
```

### 所有平台
```bash
cd sing-box
./build_all_platforms.sh
```

---

## ✅ 验证清单

### iOS
- [x] arm64 真机架构已编译
- [x] arm64 模拟器架构已编译
- [x] x86_64 模拟器架构已编译
- [x] 生产版本（无 debug）
- [x] 版本：v1.12.12

### macOS
- [x] arm64 (Apple Silicon) 已编译
- [x] x86_64 (Intel) 已编译
- [x] Universal Binary 格式
- [x] 生产版本（无 debug）
- [x] 版本：v1.12.12

---

## 📝 注意事项

1. **版本兼容性**: 确保项目代码与 sing-box v1.12.12 API 兼容
2. **生产版本**: 所有库都是生产版本，不包含 debug 符号
3. **架构完整**: 所有必要的架构都已包含
4. **备份**: 替换前建议备份旧版本
5. **Xcode**: 建议使用 Xcode 14+ 以确保完整支持

---

## 🚀 快速替换

### iOS 项目
```bash
# 备份
mv SingBoxVPN-iOS/Frameworks/Libbox.xcframework \
   SingBoxVPN-iOS/Frameworks/Libbox.xcframework.backup

# 复制新版本
cp -R sing-box/libbox_output/Libbox.xcframework \
      SingBoxVPN-iOS/Frameworks/
```

### macOS 项目
```bash
# 备份
mv macOSProject/Frameworks/Libbox.xcframework \
   macOSProject/Frameworks/Libbox.xcframework.backup

# 复制新版本
cp -R sing-box/libbox_output_macos/Libbox.xcframework \
      macOSProject/Frameworks/
```

---

## 📚 详细文档

- iOS 库文档: `libbox_output/README.md`
- macOS 库文档: `libbox_output_macos/README.md`

