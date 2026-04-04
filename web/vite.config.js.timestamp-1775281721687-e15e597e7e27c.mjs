// vite.config.js
import react from "file:///Users/lijianchao/isboyjc/remote/amux-api/web/node_modules/@vitejs/plugin-react/dist/index.mjs";
import { defineConfig, transformWithEsbuild } from "file:///Users/lijianchao/isboyjc/remote/amux-api/web/node_modules/vite/dist/node/index.js";
import pkg from "file:///Users/lijianchao/isboyjc/remote/amux-api/web/node_modules/@douyinfe/vite-plugin-semi/lib/index.js";
import path from "path";
import { codeInspectorPlugin } from "file:///Users/lijianchao/isboyjc/remote/amux-api/web/node_modules/code-inspector-plugin/dist/index.mjs";
var __vite_injected_original_dirname = "/Users/lijianchao/isboyjc/remote/amux-api/web";
var { vitePluginSemi } = pkg;
var vite_config_default = defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__vite_injected_original_dirname, "./src")
    }
  },
  plugins: [
    codeInspectorPlugin({
      bundler: "vite"
    }),
    {
      name: "treat-js-files-as-jsx",
      async transform(code, id) {
        if (!/src\/.*\.js$/.test(id)) {
          return null;
        }
        return transformWithEsbuild(code, id, {
          loader: "jsx",
          jsx: "automatic"
        });
      }
    },
    react(),
    vitePluginSemi({
      cssLayer: true
    })
  ],
  optimizeDeps: {
    force: true,
    esbuildOptions: {
      loader: {
        ".js": "jsx",
        ".json": "json"
      }
    }
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          "react-core": ["react", "react-dom", "react-router-dom"],
          "semi-ui": ["@douyinfe/semi-icons", "@douyinfe/semi-ui"],
          tools: ["axios", "history", "marked"],
          "react-components": [
            "react-dropzone",
            "react-fireworks",
            "react-telegram-login",
            "react-toastify",
            "react-turnstile"
          ],
          i18n: [
            "i18next",
            "react-i18next",
            "i18next-browser-languagedetector"
          ]
        }
      }
    }
  },
  server: {
    host: "0.0.0.0",
    proxy: {
      "/api": {
        target: "http://localhost:3000",
        changeOrigin: true
      },
      "/mj": {
        target: "http://localhost:3000",
        changeOrigin: true
      },
      "/pg": {
        target: "http://localhost:3000",
        changeOrigin: true
      }
    }
  }
});
export {
  vite_config_default as default
};
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidml0ZS5jb25maWcuanMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbImNvbnN0IF9fdml0ZV9pbmplY3RlZF9vcmlnaW5hbF9kaXJuYW1lID0gXCIvVXNlcnMvbGlqaWFuY2hhby9pc2JveWpjL3JlbW90ZS9hbXV4LWFwaS93ZWJcIjtjb25zdCBfX3ZpdGVfaW5qZWN0ZWRfb3JpZ2luYWxfZmlsZW5hbWUgPSBcIi9Vc2Vycy9saWppYW5jaGFvL2lzYm95amMvcmVtb3RlL2FtdXgtYXBpL3dlYi92aXRlLmNvbmZpZy5qc1wiO2NvbnN0IF9fdml0ZV9pbmplY3RlZF9vcmlnaW5hbF9pbXBvcnRfbWV0YV91cmwgPSBcImZpbGU6Ly8vVXNlcnMvbGlqaWFuY2hhby9pc2JveWpjL3JlbW90ZS9hbXV4LWFwaS93ZWIvdml0ZS5jb25maWcuanNcIjsvKlxuQ29weXJpZ2h0IChDKSAyMDI1IFF1YW50dW1Ob3VzXG5cblRoaXMgcHJvZ3JhbSBpcyBmcmVlIHNvZnR3YXJlOiB5b3UgY2FuIHJlZGlzdHJpYnV0ZSBpdCBhbmQvb3IgbW9kaWZ5XG5pdCB1bmRlciB0aGUgdGVybXMgb2YgdGhlIEdOVSBBZmZlcm8gR2VuZXJhbCBQdWJsaWMgTGljZW5zZSBhc1xucHVibGlzaGVkIGJ5IHRoZSBGcmVlIFNvZnR3YXJlIEZvdW5kYXRpb24sIGVpdGhlciB2ZXJzaW9uIDMgb2YgdGhlXG5MaWNlbnNlLCBvciAoYXQgeW91ciBvcHRpb24pIGFueSBsYXRlciB2ZXJzaW9uLlxuXG5UaGlzIHByb2dyYW0gaXMgZGlzdHJpYnV0ZWQgaW4gdGhlIGhvcGUgdGhhdCBpdCB3aWxsIGJlIHVzZWZ1bCxcbmJ1dCBXSVRIT1VUIEFOWSBXQVJSQU5UWTsgd2l0aG91dCBldmVuIHRoZSBpbXBsaWVkIHdhcnJhbnR5IG9mXG5NRVJDSEFOVEFCSUxJVFkgb3IgRklUTkVTUyBGT1IgQSBQQVJUSUNVTEFSIFBVUlBPU0UuIFNlZSB0aGVcbkdOVSBBZmZlcm8gR2VuZXJhbCBQdWJsaWMgTGljZW5zZSBmb3IgbW9yZSBkZXRhaWxzLlxuXG5Zb3Ugc2hvdWxkIGhhdmUgcmVjZWl2ZWQgYSBjb3B5IG9mIHRoZSBHTlUgQWZmZXJvIEdlbmVyYWwgUHVibGljIExpY2Vuc2VcbmFsb25nIHdpdGggdGhpcyBwcm9ncmFtLiBJZiBub3QsIHNlZSA8aHR0cHM6Ly93d3cuZ251Lm9yZy9saWNlbnNlcy8+LlxuXG5Gb3IgY29tbWVyY2lhbCBsaWNlbnNpbmcsIHBsZWFzZSBjb250YWN0IHN1cHBvcnRAcXVhbnR1bW5vdXMuY29tXG4qL1xuXG5pbXBvcnQgcmVhY3QgZnJvbSAnQHZpdGVqcy9wbHVnaW4tcmVhY3QnO1xuaW1wb3J0IHsgZGVmaW5lQ29uZmlnLCB0cmFuc2Zvcm1XaXRoRXNidWlsZCB9IGZyb20gJ3ZpdGUnO1xuaW1wb3J0IHBrZyBmcm9tICdAZG91eWluZmUvdml0ZS1wbHVnaW4tc2VtaSc7XG5pbXBvcnQgcGF0aCBmcm9tICdwYXRoJztcbmltcG9ydCB7IGNvZGVJbnNwZWN0b3JQbHVnaW4gfSBmcm9tICdjb2RlLWluc3BlY3Rvci1wbHVnaW4nO1xuY29uc3QgeyB2aXRlUGx1Z2luU2VtaSB9ID0gcGtnO1xuXG4vLyBodHRwczovL3ZpdGVqcy5kZXYvY29uZmlnL1xuZXhwb3J0IGRlZmF1bHQgZGVmaW5lQ29uZmlnKHtcbiAgcmVzb2x2ZToge1xuICAgIGFsaWFzOiB7XG4gICAgICAnQCc6IHBhdGgucmVzb2x2ZShfX2Rpcm5hbWUsICcuL3NyYycpLFxuICAgIH0sXG4gIH0sXG4gIHBsdWdpbnM6IFtcbiAgICBjb2RlSW5zcGVjdG9yUGx1Z2luKHtcbiAgICAgIGJ1bmRsZXI6ICd2aXRlJyxcbiAgICB9KSxcbiAgICB7XG4gICAgICBuYW1lOiAndHJlYXQtanMtZmlsZXMtYXMtanN4JyxcbiAgICAgIGFzeW5jIHRyYW5zZm9ybShjb2RlLCBpZCkge1xuICAgICAgICBpZiAoIS9zcmNcXC8uKlxcLmpzJC8udGVzdChpZCkpIHtcbiAgICAgICAgICByZXR1cm4gbnVsbDtcbiAgICAgICAgfVxuXG4gICAgICAgIC8vIFVzZSB0aGUgZXhwb3NlZCB0cmFuc2Zvcm0gZnJvbSB2aXRlLCBpbnN0ZWFkIG9mIGRpcmVjdGx5XG4gICAgICAgIC8vIHRyYW5zZm9ybWluZyB3aXRoIGVzYnVpbGRcbiAgICAgICAgcmV0dXJuIHRyYW5zZm9ybVdpdGhFc2J1aWxkKGNvZGUsIGlkLCB7XG4gICAgICAgICAgbG9hZGVyOiAnanN4JyxcbiAgICAgICAgICBqc3g6ICdhdXRvbWF0aWMnLFxuICAgICAgICB9KTtcbiAgICAgIH0sXG4gICAgfSxcbiAgICByZWFjdCgpLFxuICAgIHZpdGVQbHVnaW5TZW1pKHtcbiAgICAgIGNzc0xheWVyOiB0cnVlLFxuICAgIH0pLFxuICBdLFxuICBvcHRpbWl6ZURlcHM6IHtcbiAgICBmb3JjZTogdHJ1ZSxcbiAgICBlc2J1aWxkT3B0aW9uczoge1xuICAgICAgbG9hZGVyOiB7XG4gICAgICAgICcuanMnOiAnanN4JyxcbiAgICAgICAgJy5qc29uJzogJ2pzb24nLFxuICAgICAgfSxcbiAgICB9LFxuICB9LFxuICBidWlsZDoge1xuICAgIHJvbGx1cE9wdGlvbnM6IHtcbiAgICAgIG91dHB1dDoge1xuICAgICAgICBtYW51YWxDaHVua3M6IHtcbiAgICAgICAgICAncmVhY3QtY29yZSc6IFsncmVhY3QnLCAncmVhY3QtZG9tJywgJ3JlYWN0LXJvdXRlci1kb20nXSxcbiAgICAgICAgICAnc2VtaS11aSc6IFsnQGRvdXlpbmZlL3NlbWktaWNvbnMnLCAnQGRvdXlpbmZlL3NlbWktdWknXSxcbiAgICAgICAgICB0b29sczogWydheGlvcycsICdoaXN0b3J5JywgJ21hcmtlZCddLFxuICAgICAgICAgICdyZWFjdC1jb21wb25lbnRzJzogW1xuICAgICAgICAgICAgJ3JlYWN0LWRyb3B6b25lJyxcbiAgICAgICAgICAgICdyZWFjdC1maXJld29ya3MnLFxuICAgICAgICAgICAgJ3JlYWN0LXRlbGVncmFtLWxvZ2luJyxcbiAgICAgICAgICAgICdyZWFjdC10b2FzdGlmeScsXG4gICAgICAgICAgICAncmVhY3QtdHVybnN0aWxlJyxcbiAgICAgICAgICBdLFxuICAgICAgICAgIGkxOG46IFtcbiAgICAgICAgICAgICdpMThuZXh0JyxcbiAgICAgICAgICAgICdyZWFjdC1pMThuZXh0JyxcbiAgICAgICAgICAgICdpMThuZXh0LWJyb3dzZXItbGFuZ3VhZ2VkZXRlY3RvcicsXG4gICAgICAgICAgXSxcbiAgICAgICAgfSxcbiAgICAgIH0sXG4gICAgfSxcbiAgfSxcbiAgc2VydmVyOiB7XG4gICAgaG9zdDogJzAuMC4wLjAnLFxuICAgIHByb3h5OiB7XG4gICAgICAnL2FwaSc6IHtcbiAgICAgICAgdGFyZ2V0OiAnaHR0cDovL2xvY2FsaG9zdDozMDAwJyxcbiAgICAgICAgY2hhbmdlT3JpZ2luOiB0cnVlLFxuICAgICAgfSxcbiAgICAgICcvbWonOiB7XG4gICAgICAgIHRhcmdldDogJ2h0dHA6Ly9sb2NhbGhvc3Q6MzAwMCcsXG4gICAgICAgIGNoYW5nZU9yaWdpbjogdHJ1ZSxcbiAgICAgIH0sXG4gICAgICAnL3BnJzoge1xuICAgICAgICB0YXJnZXQ6ICdodHRwOi8vbG9jYWxob3N0OjMwMDAnLFxuICAgICAgICBjaGFuZ2VPcmlnaW46IHRydWUsXG4gICAgICB9LFxuICAgIH0sXG4gIH0sXG59KTtcbiJdLAogICJtYXBwaW5ncyI6ICI7QUFtQkEsT0FBTyxXQUFXO0FBQ2xCLFNBQVMsY0FBYyw0QkFBNEI7QUFDbkQsT0FBTyxTQUFTO0FBQ2hCLE9BQU8sVUFBVTtBQUNqQixTQUFTLDJCQUEyQjtBQXZCcEMsSUFBTSxtQ0FBbUM7QUF3QnpDLElBQU0sRUFBRSxlQUFlLElBQUk7QUFHM0IsSUFBTyxzQkFBUSxhQUFhO0FBQUEsRUFDMUIsU0FBUztBQUFBLElBQ1AsT0FBTztBQUFBLE1BQ0wsS0FBSyxLQUFLLFFBQVEsa0NBQVcsT0FBTztBQUFBLElBQ3RDO0FBQUEsRUFDRjtBQUFBLEVBQ0EsU0FBUztBQUFBLElBQ1Asb0JBQW9CO0FBQUEsTUFDbEIsU0FBUztBQUFBLElBQ1gsQ0FBQztBQUFBLElBQ0Q7QUFBQSxNQUNFLE1BQU07QUFBQSxNQUNOLE1BQU0sVUFBVSxNQUFNLElBQUk7QUFDeEIsWUFBSSxDQUFDLGVBQWUsS0FBSyxFQUFFLEdBQUc7QUFDNUIsaUJBQU87QUFBQSxRQUNUO0FBSUEsZUFBTyxxQkFBcUIsTUFBTSxJQUFJO0FBQUEsVUFDcEMsUUFBUTtBQUFBLFVBQ1IsS0FBSztBQUFBLFFBQ1AsQ0FBQztBQUFBLE1BQ0g7QUFBQSxJQUNGO0FBQUEsSUFDQSxNQUFNO0FBQUEsSUFDTixlQUFlO0FBQUEsTUFDYixVQUFVO0FBQUEsSUFDWixDQUFDO0FBQUEsRUFDSDtBQUFBLEVBQ0EsY0FBYztBQUFBLElBQ1osT0FBTztBQUFBLElBQ1AsZ0JBQWdCO0FBQUEsTUFDZCxRQUFRO0FBQUEsUUFDTixPQUFPO0FBQUEsUUFDUCxTQUFTO0FBQUEsTUFDWDtBQUFBLElBQ0Y7QUFBQSxFQUNGO0FBQUEsRUFDQSxPQUFPO0FBQUEsSUFDTCxlQUFlO0FBQUEsTUFDYixRQUFRO0FBQUEsUUFDTixjQUFjO0FBQUEsVUFDWixjQUFjLENBQUMsU0FBUyxhQUFhLGtCQUFrQjtBQUFBLFVBQ3ZELFdBQVcsQ0FBQyx3QkFBd0IsbUJBQW1CO0FBQUEsVUFDdkQsT0FBTyxDQUFDLFNBQVMsV0FBVyxRQUFRO0FBQUEsVUFDcEMsb0JBQW9CO0FBQUEsWUFDbEI7QUFBQSxZQUNBO0FBQUEsWUFDQTtBQUFBLFlBQ0E7QUFBQSxZQUNBO0FBQUEsVUFDRjtBQUFBLFVBQ0EsTUFBTTtBQUFBLFlBQ0o7QUFBQSxZQUNBO0FBQUEsWUFDQTtBQUFBLFVBQ0Y7QUFBQSxRQUNGO0FBQUEsTUFDRjtBQUFBLElBQ0Y7QUFBQSxFQUNGO0FBQUEsRUFDQSxRQUFRO0FBQUEsSUFDTixNQUFNO0FBQUEsSUFDTixPQUFPO0FBQUEsTUFDTCxRQUFRO0FBQUEsUUFDTixRQUFRO0FBQUEsUUFDUixjQUFjO0FBQUEsTUFDaEI7QUFBQSxNQUNBLE9BQU87QUFBQSxRQUNMLFFBQVE7QUFBQSxRQUNSLGNBQWM7QUFBQSxNQUNoQjtBQUFBLE1BQ0EsT0FBTztBQUFBLFFBQ0wsUUFBUTtBQUFBLFFBQ1IsY0FBYztBQUFBLE1BQ2hCO0FBQUEsSUFDRjtBQUFBLEVBQ0Y7QUFDRixDQUFDOyIsCiAgIm5hbWVzIjogW10KfQo=
