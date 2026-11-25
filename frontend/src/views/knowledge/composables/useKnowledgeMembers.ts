import { computed, ref, type Ref } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';
import { useAuthStore } from '@/stores/auth';
import {
  addKnowledgeBaseMember,
  listKnowledgeBaseMembers,
  removeKnowledgeBaseMember,
  type KnowledgeBaseMember,
  type KnowledgeBaseMemberRole,
} from '@/api/knowledge-base/index';
import { fetchAllTenantMembers, type TenantMember } from '@/api/tenant/members';

const KB_MEMBER_ROLE_LABELS: Record<KnowledgeBaseMemberRole, string> = {
  viewer: '只读',
  contributor: '可编辑',
  admin: '管理员',
};

/**
 * Knowledge-base member management (project KBs): list/load, add, change role
 * and remove. Display names come from the tenant member directory.
 */
export function useKnowledgeMembers(options: { kbId: Ref<string> }) {
  const { kbId } = options;
  const authStore = useAuthStore();

  const membersDialogVisible = ref(false);
  const kbMembers = ref<KnowledgeBaseMember[]>([]);
  const kbMembersLoading = ref(false);
  const tenantMembers = ref<TenantMember[]>([]);
  const tenantMembersLoading = ref(false);
  const memberSavingUserId = ref('');
  const addMemberUserId = ref('');
  const addMemberRole = ref<KnowledgeBaseMemberRole>('viewer');
  const addMemberSubmitting = ref(false);

  const memberRoleOptions = (Object.keys(KB_MEMBER_ROLE_LABELS) as KnowledgeBaseMemberRole[]).map(
    (value) => ({ value, label: KB_MEMBER_ROLE_LABELS[value] }),
  );
  const tenantMemberMap = computed(
    () => new Map(tenantMembers.value.map((member) => [member.user_id, member])),
  );
  const kbMemberDisplayName = (userId: string) => {
    const member = tenantMemberMap.value.get(userId);
    return member?.username || member?.email || userId;
  };
  const kbMemberDisplayEmail = (userId: string) => {
    const member = tenantMemberMap.value.get(userId);
    return member?.email || '';
  };
  // 已在库成员中的人不再出现在「添加成员」下拉里。
  const addMemberOptions = computed(() =>
    tenantMembers.value
      .filter((member) => !kbMembers.value.some((kbMember) => kbMember.user_id === member.user_id))
      .map((member) => ({
        value: member.user_id,
        label: member.username ? `${member.username}（${member.email}）` : member.email,
      })),
  );

  const loadKbMembers = async () => {
    if (!kbId.value) return;
    kbMembersLoading.value = true;
    try {
      const resp = await listKnowledgeBaseMembers(kbId.value);
      kbMembers.value = resp?.data || [];
    } catch (error: any) {
      MessagePlugin.error(error?.message || '加载成员失败');
    } finally {
      kbMembersLoading.value = false;
    }
  };

  const loadTenantMembers = async () => {
    const tenantId = authStore.effectiveTenantId;
    if (!tenantId) return;
    tenantMembersLoading.value = true;
    try {
      tenantMembers.value = await fetchAllTenantMembers(Number(tenantId));
    } catch (error: any) {
      MessagePlugin.error(error?.message || '加载空间成员失败');
    } finally {
      tenantMembersLoading.value = false;
    }
  };

  const openMembersDialog = () => {
    membersDialogVisible.value = true;
    addMemberUserId.value = '';
    addMemberRole.value = 'viewer';
    void Promise.all([loadKbMembers(), loadTenantMembers()]);
  };

  const handleMemberRoleChange = async (member: KnowledgeBaseMember, role: any) => {
    if (!kbId.value || !role || role === member.role) return;
    memberSavingUserId.value = member.user_id;
    try {
      await addKnowledgeBaseMember(kbId.value, member.user_id, role as KnowledgeBaseMemberRole);
      member.role = role as KnowledgeBaseMemberRole;
      MessagePlugin.success('成员角色已更新');
    } catch (error: any) {
      MessagePlugin.error(error?.message || '更新成员角色失败');
      await loadKbMembers();
    } finally {
      memberSavingUserId.value = '';
    }
  };

  const handleRemoveKbMember = async (member: KnowledgeBaseMember) => {
    if (!kbId.value) return;
    memberSavingUserId.value = member.user_id;
    try {
      await removeKnowledgeBaseMember(kbId.value, member.user_id);
      kbMembers.value = kbMembers.value.filter((item) => item.user_id !== member.user_id);
      MessagePlugin.success('已移除成员');
    } catch (error: any) {
      MessagePlugin.error(error?.message || '移除成员失败');
    } finally {
      memberSavingUserId.value = '';
    }
  };

  const handleAddKbMember = async () => {
    if (!kbId.value || !addMemberUserId.value) return;
    addMemberSubmitting.value = true;
    try {
      await addKnowledgeBaseMember(kbId.value, addMemberUserId.value, addMemberRole.value);
      MessagePlugin.success('已添加成员');
      addMemberUserId.value = '';
      await loadKbMembers();
    } catch (error: any) {
      MessagePlugin.error(error?.message || '添加成员失败');
    } finally {
      addMemberSubmitting.value = false;
    }
  };

  return {
    membersDialogVisible,
    kbMembers,
    kbMembersLoading,
    tenantMembers,
    tenantMembersLoading,
    memberSavingUserId,
    addMemberUserId,
    addMemberRole,
    addMemberSubmitting,
    memberRoleOptions,
    kbMemberDisplayName,
    kbMemberDisplayEmail,
    addMemberOptions,
    openMembersDialog,
    loadKbMembers,
    loadTenantMembers,
    handleMemberRoleChange,
    handleRemoveKbMember,
    handleAddKbMember,
  };
}