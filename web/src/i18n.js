import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

import translationEN from './locales/en.json'
import translationZH from './locales/zh.json'

// 翻译资源
const resources = {
  en: {
    translation: translationEN
  },
  zh: {
    translation: translationZH
  }
}

// 配置 i18n（不在这里初始化，由 main.jsx 控制初始化时机）
i18n
  .use(LanguageDetector)
  .use(initReactI18next)

// 导出配置，由 main.jsx 调用 init
export const i18nConfig = {
  resources,
  supportedLngs: ['zh', 'en'],
  fallbackLng: 'zh',
  debug: true,

  interpolation: {
    escapeValue: false
  },

  detection: {
    order: ['navigator', 'htmlTag', 'querystring', 'cookie', 'localStorage'],
    caches: ['localStorage'],
    excludeCacheFor: ['cimode'],
    lookupNavigator: 'languages',
    convertDetectedLanguage: function(lang) {
      if (lang.startsWith('en')) return 'en'
      if (lang.startsWith('zh')) return 'zh'
      return lang
    }
  }
}

// 将 i18n 实例暴露到全局，方便调试
if (typeof window !== 'undefined') {
  window.i18n = i18n
}

export default i18n
