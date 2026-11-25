import { computed, type ComputedRef, type Ref } from 'vue';
import { useOrganizationStore } from '@/stores/organization';
import { useAuthStore } from '@/stores/auth';
import { permissionCanEditKB, permissionCanManageKB } from '@/utils/kbPermission';

/**
 * Capability-derived permission gates for the knowledge base detail page.
 *
 * The server-computed `capabilities` object is authoritative whenever a flag
 * is present; the legacy role/permission fallbacks keep older backends usable.
 */
export function useKnowledgePermissions(options: {
  kbId: ComputedRef<string>;
  kbInfo: Ref<any>;
  isFAQ: ComputedRef<boolean>;
}) {
  const { kbId, kbInfo, isFAQ } = options;
  const orgStore = useOrganizationStore();
  const authStore = useAuthStore();

  // Permission control: check if current user owns this KB or has edit/manage permission
  //
  // "Owner" here is "the original creator of this KB" (PR 5 introduced
  // CreatorID). The previous version compared kb.tenant_id to the active
  // tenant id, which only answers "is this KB inside our tenant" — that
  // is true even for a Viewer in someone else's tenant, so the gate
  // silently bypassed every role check below. Now we require an explicit
  // creator match, and the role-aware fallbacks below decide whether a
  // non-creator may edit / manage.
  const isOwner = computed(() => {
    if (!kbInfo.value) return false;
    const creatorId = (kbInfo.value as any).creator_id || '';
    const userId = authStore.user?.id || '';
    // creator_id may be empty for legacy KBs created before PR 5; treat
    // those as tenant-owned so the role gate applies (Admin+ can manage,
    // Viewer cannot).
    if (!creatorId) return false;
    return creatorId === userId;
  });

  // Current KB's shared record (when accessed via organization share)
  const currentSharedKb = computed(() =>
    orgStore.sharedKnowledgeBases.find((s) => s.knowledge_base?.id === kbId.value) ?? null,
  );

  // Accessed via organization share: when the KB shows up in our
  // sharedKnowledgeBases list it means we reached it through a shared space,
  // not because we own/manage it in our tenant. In that case the user's local
  // tenant role does NOT grant edit/manage — only the share grant does.
  // Without this guard a local tenant Admin would see edit/upload entries on
  // a read-only shared KB and get 403'd by the backend on click.
  //
  // Note: tenant_id comparison alone is unreliable — a user can be a member of
  // both the source and receiving tenants, and currentTenantId reflects the
  // active switcher rather than "how this KB became visible to me". Presence
  // in the share list is the authoritative signal.
  const isViaShare = computed(() => !!currentSharedKb.value);

  // Effective permission: from direct org share list or from GET /knowledge-bases/:id (e.g. agent-visible KB).
  // Declared first: the canEdit / canManage gates below treat an explicit
  // cross-tenant grant as the single source of truth.
  const effectiveKBPermission = computed(() => orgStore.getKBPermission(kbId.value) || kbInfo.value?.my_permission || '');
  // 服务端 GET /knowledge-bases/:id 计算的能力契约。capabilities 同时承载
  // 存储/功能开关（vector/keyword/...），这里只读取授权布尔键；键不存在时
  // 回退到旧的前端推导，兼容尚未升级的后端。
  const kbCapabilities = computed<Record<string, unknown>>(() => kbInfo.value?.capabilities ?? {});
  const capabilityBool = (name: string): boolean | undefined => {
    const value = kbCapabilities.value?.[name];
    return typeof value === 'boolean' ? value : undefined;
  };
  // 服务端 requireKBAdmin 的镜像：只有库管理员能改成员；只读成员只看到名单。
  const canManageKbMembers = computed(() => {
    const fromCapabilities = capabilityBool('manage_members');
    if (fromCapabilities !== undefined) return fromCapabilities;
    return kbInfo.value?.can_manage_members === true;
  });

  // Can edit: when accessed via an organization share, ONLY the share grant
  // counts — even if the current user happens to be the original creator of the
  // KB. The backend's RBAC middleware authorizes based on the active
  // tenant, not on creator_id, so a creator viewing their own KB from a
  // different tenant context will be 403'd on write. Otherwise: KB creator
  // (any role) or tenant Admin+ in the home tenant.
  //
  // A resolved cross-tenant permission (org share or shared-agent visibility,
  // already capped server-side by effective = min(share, org_role, tenant_role))
  // outranks every local signal: without this priority rule a personal-workspace
  // admin browsing a read-only shared KB saw edit entries that only led to 403s
  // (#3098).
  //
  // hasRole('contributor') is intentionally NOT here — being a Contributor
  // in a tenant does not by itself grant edit on someone else's KB.
  const canEdit = computed(() => {
    const fromCapabilities = capabilityBool('edit');
    if (fromCapabilities !== undefined) return fromCapabilities;
    const permission = effectiveKBPermission.value;
    if (permission) return permissionCanEditKB(permission);
    if (isViaShare.value) return orgStore.canEditKB(kbId.value, false);
    if (isOwner.value) return true;
    if (authStore.hasRole('admin')) return true;
    return orgStore.canEditKB(kbId.value, false);
  });

  // 上传（新增文档）比编辑/删除更宽：同空间 contributor 也可以投稿，
  // 与后端 /knowledge-bases/:id/knowledge/file 的有效库角色闸门对齐。
  // 跨空间分享仍只认显式 grant，避免借 tenant 角色越权写别人的库。
  const canUpload = computed(() => {
    const fromCapabilities = capabilityBool('upload');
    if (fromCapabilities !== undefined) return fromCapabilities;
    if (canEdit.value) return true;
    if (isViaShare.value || isFAQ.value) return false;
    return authStore.hasRole('contributor');
  });

  // Can manage (delete, settings, etc.): same permission-first rule. For
  // shared KBs only an 'admin' share grant qualifies — editor/viewer (and
  // even being the creator viewed via share) never grant delete/settings.
  const canManage = computed(() => {
    // KB lifecycle (rename/delete/re-model) is stricter than content deletion:
    // a kb_members admin may delete documents but not the knowledge base itself.
    const fromManageKb = capabilityBool('manage_kb');
    if (fromManageKb !== undefined) return fromManageKb;
    const fromCapabilities = capabilityBool('delete');
    if (fromCapabilities !== undefined) return fromCapabilities;
    const permission = effectiveKBPermission.value;
    if (permission) return permissionCanManageKB(permission);
    if (isViaShare.value) return orgStore.canManageKB(kbId.value, false);
    if (isOwner.value) return true;
    if (authStore.hasRole('admin')) return true;
    return orgStore.canManageKB(kbId.value, false);
  });

  // The activity feed exposes owner-side actor and configuration summaries.
  // It lives in KB settings (KnowledgeBaseEditorModal) for Owner/Admin in the home tenant.

  // 单文档删除在后端走 OwnedKnowledgeKBOrAdmin（库管理员/创建者/空间管理员），
  // 因此单独消费 capabilities.delete；旧后端没有该键时退回 canEdit。
  const canDeleteKnowledge = computed(() => {
    const fromCapabilities = capabilityBool('delete');
    if (fromCapabilities !== undefined) return fromCapabilities;
    return canEdit.value;
  });

  // 移动/批量管理入口：capabilities.edit 为真时，库贡献者也可以移动、重解析和
  // 改标签；批量删除仍由后端按库管理员校验，前端保持入口可见但可能得到 403
  // （与历史行为一致）。
  const canMutateKnowledge = computed(() => {
    const fromCapabilities = capabilityBool('edit');
    if (fromCapabilities !== undefined) return fromCapabilities;
    if (!canEdit.value) return false;
    if (isViaShare.value) return true;
    if (isOwner.value) return true;
    if (authStore.hasRole('admin')) return true;
    return authStore.hasRole('contributor');
  });

  // Downloading returns the original source file, which is intentionally more
  // restrictive than viewing parsed content or using the preview tab. A tenant
  // Viewer can never download; for cross-tenant KBs the effective share
  // permission must additionally be Editor or Admin.
  const canDownloadKnowledge = computed(() => {
    // 下载走 KBAccessWrite（有效库角色 contributor 及以上），与 upload 同一档；
    // 不能复用 edit，因为空间 contributor 可下载但不能改他人库里的既有文档。
    const fromCapabilities = capabilityBool('upload');
    if (fromCapabilities !== undefined) return fromCapabilities;
    if (!authStore.hasRole('contributor')) return false;
    const permission = effectiveKBPermission.value;
    return !permission || permission === 'owner' || permission === 'admin' || permission === 'editor';
  });

  return {
    isOwner,
    currentSharedKb,
    isViaShare,
    effectiveKBPermission,
    kbCapabilities,
    capabilityBool,
    canManageKbMembers,
    canEdit,
    canUpload,
    canManage,
    canDeleteKnowledge,
    canMutateKnowledge,
    canDownloadKnowledge,
  };
}