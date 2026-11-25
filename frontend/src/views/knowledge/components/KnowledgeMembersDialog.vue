<script setup lang="ts">
import { computed } from 'vue';
import { useKnowledgeMembers } from '../composables/useKnowledgeMembers';

const props = defineProps<{
  kbId: string;
  kbInfo: any;
  canManageKbMembers: boolean;
}>();

const {
  membersDialogVisible,
  kbMembers,
  kbMembersLoading,
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
  handleMemberRoleChange,
  handleRemoveKbMember,
  handleAddKbMember,
} = useKnowledgeMembers({ kbId: computed(() => props.kbId) });

const kbScopeText = computed(() => {
  const scope = props.kbInfo?.scope;
  return scope === 'project' ? '项目' : scope === 'personal' ? '私密' : '公开';
});

defineExpose({ open: openMembersDialog });
</script>

<template>
  <t-dialog
    v-model:visible="membersDialogVisible"
    header="成员与权限"
    :footer="false"
    width="620px"
    placement="center"
    class="kb-members-dialog"
    data-guide="kb-members-dialog"
  >
    <div class="kb-members">
      <p class="kb-members-summary">
        当前知识库：<strong>{{ kbInfo?.name || '—' }}</strong>
        <span class="kb-members-scope">可见范围：{{ kbScopeText }}</span>
      </p>

      <div class="kb-members-section-title">已授权成员</div>
      <t-loading :loading="kbMembersLoading" size="small">
        <div v-if="!kbMembersLoading && kbMembers.length === 0" class="kb-members-empty">
          暂无成员授权
        </div>
        <div v-else class="kb-members-list">
          <div v-for="member in kbMembers" :key="member.user_id" class="kb-member-row">
            <div class="kb-member-identity">
              <span class="kb-member-name">{{ kbMemberDisplayName(member.user_id) }}</span>
              <span v-if="kbMemberDisplayEmail(member.user_id)" class="kb-member-email">
                {{ kbMemberDisplayEmail(member.user_id) }}
              </span>
            </div>
            <t-select
              class="kb-member-role"
              :model-value="member.role"
              :options="memberRoleOptions"
              size="small"
              :disabled="!canManageKbMembers || memberSavingUserId === member.user_id"
              @change="(value: any) => handleMemberRoleChange(member, value)"
            />
            <t-popconfirm
              v-if="canManageKbMembers"
              content="确定移除该成员？"
              @confirm="handleRemoveKbMember(member)"
            >
              <t-button
                theme="danger"
                variant="text"
                size="small"
                :disabled="memberSavingUserId === member.user_id"
              >
                移除
              </t-button>
            </t-popconfirm>
          </div>
        </div>
      </t-loading>

      <p v-if="!canManageKbMembers" class="kb-members-readonly">
        仅知识库管理员可以调整成员权限。
      </p>

      <div v-if="canManageKbMembers" class="kb-members-add">
        <div class="kb-members-section-title">添加成员</div>
        <div class="kb-members-add-row">
          <t-select
            v-model="addMemberUserId"
            class="kb-members-add-user"
            filterable
            clearable
            placeholder="从空间成员中选择"
            :options="addMemberOptions"
            :loading="tenantMembersLoading"
            data-guide="kb-members-add-user"
          />
          <t-select
            v-model="addMemberRole"
            class="kb-members-add-role"
            :options="memberRoleOptions"
            data-guide="kb-members-add-role"
          />
          <t-button
            theme="primary"
            :loading="addMemberSubmitting"
            :disabled="!addMemberUserId"
            data-guide="kb-members-add-submit"
            @click="handleAddKbMember"
          >
            添加
          </t-button>
        </div>
        <p v-if="!tenantMembersLoading && addMemberOptions.length === 0" class="kb-members-add-empty">
          空间成员都已获得授权。
        </p>
      </div>
    </div>
  </t-dialog>
</template>

<style scoped lang="less">
// 成员与权限弹窗
.kb-members-dialog {
  // TDesign 默认固定 620px，窄屏时不要撑出视口。
  max-width: calc(100vw - 32px);
}

.kb-members {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-top: 4px;
  color: var(--td-text-color-primary);
}

.kb-members-summary {
  margin: 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: var(--app-text-base);
  color: var(--td-text-color-secondary);

  strong {
    color: var(--td-text-color-primary);
    font-weight: 600;
  }
}

.kb-members-scope {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
  font-size: var(--app-text-xs);
}

.kb-members-section-title {
  font-size: var(--app-text-sm);
  font-weight: 600;
  color: var(--td-text-color-secondary);
}

.kb-members-list {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  max-height: 300px;
  overflow-y: auto;
}

.kb-member-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 108px 56px;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.kb-member-identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.kb-member-name {
  font-size: var(--app-text-base);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kb-member-email {
  font-size: var(--app-text-xs);
  color: var(--td-text-color-placeholder);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kb-members-empty {
  padding: 28px 12px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 8px;
  text-align: center;
  font-size: var(--app-text-base);
  color: var(--td-text-color-placeholder);
}

.kb-members-readonly {
  margin: 16px 0 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  color: var(--td-text-color-secondary, #666);
  font-size: 13px;
}

.kb-members-add {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 4px;
  border-top: 1px solid var(--td-component-stroke);
}

.kb-members-add-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.kb-members-add-user {
  flex: 1;
  min-width: 0;
}

.kb-members-add-role {
  width: 108px;
  flex-shrink: 0;
}

.kb-members-add-empty {
  margin: 0;
  font-size: var(--app-text-xs);
  color: var(--td-text-color-placeholder);
}
</style>