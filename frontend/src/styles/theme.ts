/**
 * Ant Design Vue 4 主题 token — 与 styles/variables.scss 保持同一套视觉语言
 * 中性优先：控件描边清晰、圆角克制、主色只用于可交互/主操作
 */
import type { ThemeConfig } from 'ant-design-vue/es/config-provider/context'

export const antdTheme: ThemeConfig = {
  token: {
    colorPrimary: '#2563eb',
    colorSuccess: '#1a7f37',
    colorWarning: '#9a6700',
    colorError: '#cf222e',
    colorInfo: '#2563eb',

    colorBgLayout: '#f6f8fa',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    colorBgSpotlight: '#1f2328',

    colorText: '#1f2328',
    colorTextSecondary: '#59636e',
    colorTextTertiary: '#8c959f',
    colorTextQuaternary: '#afb8c1',

    colorBorder: '#d0d7de',
    colorBorderSecondary: '#e6e9ed',
    colorSplit: '#e6e9ed',

    borderRadius: 6,
    borderRadiusLG: 8,
    borderRadiusSM: 4,
    borderRadiusXS: 2,

    fontSize: 13,
    fontSizeSM: 12,
    fontSizeLG: 14,
    fontSizeHeading1: 24,
    fontSizeHeading2: 20,
    fontSizeHeading3: 16,
    fontSizeHeading4: 14,
    fontSizeHeading5: 13,

    controlHeight: 32,
    controlHeightSM: 24,
    controlHeightLG: 36,

    boxShadow: '0 1px 2px rgba(31,35,40,0.04)',
    boxShadowSecondary: '0 8px 24px rgba(31,35,40,0.12)',

    motionDurationFast: '0.12s',
    motionDurationMid: '0.18s',
    wireframe: false,
  },
  // 组件级 token 字段随 antd 版本变动较多，这里只保留稳定全局 token；
  // 表格/菜单等细节由 global.scss 覆盖
  components: {
    Table: {
      headerBg: '#f6f8fa',
      headerColor: '#59636e',
      rowHoverBg: '#f6f8fa',
      borderColor: '#e6e9ed',
      headerSplit: false,
    },
    Button: {
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Tabs: {
      inkBarColor: '#2563eb',
    },
    Select: {
      optionSelectedBg: '#eff6ff',
    },
  } as ThemeConfig['components'],
}

export default antdTheme
