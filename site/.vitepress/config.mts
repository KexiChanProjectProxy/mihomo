import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Mihomo',
  description: 'Another Mihomo Kernel',
  lastUpdated: true,

  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/introduction', activeMatch: '/guide/' },
      { text: 'Reference', link: '/reference/config-example', activeMatch: '/reference/' },
      { text: 'GitHub', link: 'https://github.com/MetaCubeX/mihomo' }
    ],

    sidebar: {
      '/guide/': [
        {
          text: 'Guide',
          items: [
            { text: 'Introduction', link: '/guide/introduction' },
            { text: 'Development', link: '/guide/development' },
            { text: 'Release Workflow', link: '/guide/release-workflow' },
            { text: 'Cloudflare Pages', link: '/guide/cloudflare-pages' }
          ]
        }
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'Configuration Example', link: '/reference/config-example' },
            { text: 'Testing', link: '/reference/testing' }
          ]
        }
      ]
    },

    footer: {
      message: 'Released under the GPL-3.0 License.',
      copyright: 'Copyright © 2024 MetaCubeX'
    }
  }
})
