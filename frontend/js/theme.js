// ===== 主题切换逻辑 =====

function useTheme() {
    const { ref } = Vue;

    const isDarkTheme = ref(localStorage.getItem('theme') === 'dark');

    function applyTheme(dark) {
        document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
        document.body.setAttribute('data-theme', dark ? 'dark' : 'light');
        // 动态注入 Element Plus popper 暗色样式（解决 teleport 弹出层白色背景问题）
        let darkPopperStyle = document.getElementById('dark-popper-style');
        if (dark) {
            if (!darkPopperStyle) {
                darkPopperStyle = document.createElement('style');
                darkPopperStyle.id = 'dark-popper-style';
                document.head.appendChild(darkPopperStyle);
            }
            darkPopperStyle.textContent = `
                .el-popper, .el-select-dropdown, .el-popper.is-light, .el-popper.is-pure {
                    background: #1a1b2e !important;
                    border-color: #2d2f45 !important;
                }
                .el-popper .el-popper__arrow::before {
                    background: #1a1b2e !important;
                    border-color: #2d2f45 !important;
                }
                .el-select-dropdown__item {
                    color: #e4e6f0 !important;
                }
                .el-select-dropdown__item:hover,
                .el-select-dropdown__item.hover,
                .el-select-dropdown__item.is-hovering {
                    background: #252740 !important;
                }
                .el-select-dropdown__item.is-selected,
                .el-select-dropdown__item.selected {
                    color: #6d8cf8 !important;
                    font-weight: 700;
                }
                .el-select-group__title {
                    color: #6b6f85 !important;
                }
                .el-select-group__wrap::after {
                    background: #2d2f45 !important;
                }
                .el-scrollbar__thumb {
                    background: #3a3d55 !important;
                }
            `;
        } else {
            if (darkPopperStyle) {
                darkPopperStyle.remove();
            }
        }
    }

    function toggleTheme() {
        isDarkTheme.value = !isDarkTheme.value;
        localStorage.setItem('theme', isDarkTheme.value ? 'dark' : 'light');
        applyTheme(isDarkTheme.value);
    }

    // 初始化主题
    applyTheme(isDarkTheme.value);

    return {
        isDarkTheme,
        toggleTheme,
        applyTheme,
    };
}
