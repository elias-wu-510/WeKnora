import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import router from "./router";
import TDesign from "tdesign-vue-next";
// 引入组件库的少量全局样式变量
import "tdesign-vue-next/dist/tdesign.css";
import "@/assets/theme/theme.css";
import "@/assets/theme/tdesign-overrides.less";
import "@/assets/dropdown-menu.less";
import "@/components/css/chat-hljs-dark.less";
// vue-virtual-scroller ships its own tiny stylesheet — required for
// RecycleScroller/DynamicScroller to size their viewport correctly.
// Without it the scroller computes 0 height and renders no items.
import "vue-virtual-scroller/dist/vue-virtual-scroller.css";
import i18n from "./i18n";
import { initTheme } from "@/composables/useTheme";
import { initFont } from "@/composables/useFont";
import { installTDesignIconOfflineGuard } from "@/utils/tdesign-icon-offline";
import { installAutofillGuard } from "@/utils/disable-autofill";
import { useAuthStore } from "@/stores/auth";
import {
  isCKnowledgeConsole,
  preferredTenant,
  rememberPreferredTenant,
  resolveConsoleBootSelection,
} from "@/utils/ckConsole";

// 必须在 Vue 组件挂载之前执行，避免 tdesign-icons 运行时请求 tdesign.gtimg.com
installTDesignIconOfflineGuard();

initTheme();
initFont();

/**
 * C-Knowledge 管理台由 Senior AI 以 `?tenant=<id>` 指定要进入的共享空间。
 * 必须在 auth store 初始化之前落到 localStorage：首个 /auth/me 会据此带
 * X-Tenant-ID，否则会落回用户的个人空间（症状：管理台「全部 0」）。
 * 只接受正整数；服务端仍会校验 membership，无权访问时接口照常拒绝。
 */
function applyTenantHint(): void {
  try {
    const url = new URL(window.location.href);
    const raw = (url.searchParams.get("tenant") || "").trim();
    if (!/^\d+$/.test(raw) || raw === "0") {
      // 回调页、刷新后的控制台地址上没有 tenant 参数：只要还有登录态，
      // 就按上次记住的共享空间恢复，避免落回个人空间后显示「全部 0」。
      const remembered = preferredTenant();
      if (remembered && localStorage.getItem("weknora_token") && !localStorage.getItem("weknora_selected_tenant_id")) {
        localStorage.setItem("weknora_selected_tenant_id", String(remembered));
      }
      return;
    }
    localStorage.setItem("weknora_selected_tenant_id", raw);
    rememberPreferredTenant(Number(raw));
    url.searchParams.delete("tenant");
    const query = url.searchParams.toString();
    window.history.replaceState({}, document.title, `${url.pathname}${query ? `?${query}` : ""}${url.hash}`);
  } catch {
    /* URL 解析失败时忽略，按默认空间启动 */
  }
}

async function bootstrap() {
  applyTenantHint();
  const app = createApp(App);

  // 全局错误处理：捕获未处理的组件错误，防止白屏
  app.config.errorHandler = (err, instance, info) => {
    console.error("[WeKnora] Unhandled Vue error:", err, "\nComponent:", instance, "\nInfo:", info);
  };

  app.use(TDesign);
  const pinia = createPinia();
  app.use(pinia);

  // Capabilities (can_create_tenant, auto_accept_invitation) are not cached
  // in localStorage — reconcile once before first paint when a session exists.
  const authStore = useAuthStore();
  if (localStorage.getItem("weknora_token")) {
    let restored = false;
    try {
      restored = await authStore.refreshFromAuthMe();
    } catch {
      // best-effort; capabilities stay at defaults until the next refresh
    }
    // C-Knowledge 独立入口：成员关系到手后补一次空间选择。没有 ?tenant=
    // 也没有历史偏好时默认落到共享空间，让首个知识库请求就带对
    // X-Tenant-ID（否则会落在个人空间、首页显示「全部 0」）。
    if (restored && isCKnowledgeConsole()) {
      const selection = resolveConsoleBootSelection({
        activeTenantId: authStore.tenant?.id ?? null,
        activeTenantName: authStore.tenant?.name ?? null,
        homeTenantId: authStore.user?.tenant_id ?? null,
        memberships: authStore.memberships,
      });
      authStore.setSelectedTenant(selection.id, selection.name);
    }
  }

  app.use(router);
  app.use(i18n);

  // 等首屏路由（含导航守卫、Lite 自动登录）完成后再挂载，避免先闪默认页再跳转
  await router.isReady();
  app.mount("#app");
  installAutofillGuard();
}

bootstrap();
