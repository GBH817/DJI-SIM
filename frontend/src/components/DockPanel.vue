<template>
  <div class="dock-panel">
    <div class="dock-toolbar">
      <span class="dock-count">{{ docks.length }} 台机巢</span>
      <button class="add-btn" @click="openAdd">＋ 添加机巢</button>
    </div>

    <div v-if="docks.length === 0" class="empty">暂无注册机巢，点击「添加机巢」注册</div>

    <div v-for="d in docks" :key="d.sn" class="dock-item">
      <div class="dock-item-top">
        <span class="status-dot" :class="d.status"></span>
        <span class="dock-sn">{{ d.sn }}</span>
        <span class="status-text" :class="d.status">{{ d.status === 'online' ? '在线' : '离线' }}</span>
      </div>
      <div class="dock-broker">{{ d.broker }}</div>
      <div v-if="d.droneSn" class="dock-drone-sn">无人机 SN：{{ d.droneSn }}</div>
      <div v-if="d.error" class="dock-error">{{ d.error }}</div>
      <div class="dock-actions">
        <button class="act-btn" @click="openEdit(d)">编辑</button>
        <button class="act-btn" @click="reconnect(d)">重连</button>
        <button class="act-btn danger" @click="remove(d)">删除</button>
      </div>
    </div>

    <!-- 注册/编辑对话框 -->
    <div v-if="dialogVisible" class="modal-overlay" @click.self="close">
      <div class="modal">
        <h3>{{ editing ? '编辑机巢' : '添加机巢' }}</h3>

        <div class="form-row">
          <label>SN（机巢序列号）</label>
          <input
            v-model="form.sn"
            :disabled="!!editing"
            :placeholder="'大写英文 + 数字，如 DOCK123456'"
            @input="form.sn = form.sn.toUpperCase()"
          />
        </div>

        <div v-if="editing" class="form-row">
          <label>无人机 SN（自动生成）</label>
          <input :value="editing.droneSn" disabled />
        </div>

        <div class="form-row">
          <label>Org ID（组织 ID）</label>
          <input v-model="form.orgId" placeholder="如 4" />
        </div>

        <div class="form-row">
          <label>Binding Code（绑定码）</label>
          <input v-model="form.bindingCode" placeholder="如 acus2025" />
        </div>

        <div class="form-row">
          <label>MQTT Broker 地址</label>
          <input v-model="form.broker" placeholder="tcp://192.168.8.203:1883" />
        </div>

        <div class="form-row">
          <label>用户名</label>
          <input v-model="form.username" placeholder="dji_dock" />
        </div>

        <div class="form-row">
          <label>密码</label>
          <div class="password-field">
            <input
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              :placeholder="editing ? '留空则保持不变' : 'power_your_dreams'"
            />
            <button type="button" class="toggle-eye" @click="showPassword = !showPassword">
              {{ showPassword ? '隐藏' : '显示' }}
            </button>
          </div>
        </div>

        <div v-if="error" class="error">{{ error }}</div>

        <div class="modal-actions">
          <button class="cancel-btn" @click="close">取消</button>
          <button class="save-btn" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import axios from 'axios'

interface DockInfo {
  sn: string
  droneSn: string
  broker: string
  username: string
  orgId: string
  bindingCode: string
  status: string
  error?: string
}

const docks = ref<DockInfo[]>([])
const dialogVisible = ref(false)
const editing = ref<DockInfo | null>(null)
const saving = ref(false)
const error = ref('')
const showPassword = ref(false)

const form = ref({ sn: '', broker: '', username: 'dji_dock', password: 'power_your_dreams', orgId: '', bindingCode: '' })

async function loadDocks() {
  try {
    const res = await axios.get('/api/docks')
    if (res.data.success) {
      docks.value = res.data.data || []
    }
  } catch (e) {
    console.warn('加载机巢列表失败:', e)
  }
}

function openAdd() {
  editing.value = null
  form.value = { sn: '', broker: '', username: 'dji_dock', password: 'power_your_dreams', orgId: '', bindingCode: '' }
  error.value = ''
  showPassword.value = false
  dialogVisible.value = true
}

function openEdit(d: DockInfo) {
  editing.value = d
  form.value = { sn: d.sn, broker: d.broker, username: d.username, password: '', orgId: d.orgId || '', bindingCode: d.bindingCode || '' }
  error.value = ''
  showPassword.value = false
  dialogVisible.value = true
}

function close() {
  dialogVisible.value = false
  error.value = ''
}

async function save() {
  error.value = ''
  const sn = form.value.sn.trim().toUpperCase()
  const broker = form.value.broker.trim()

  if (!/^[A-Z0-9]{6,32}$/.test(sn)) {
    error.value = 'SN 需为 6-32 位大写英文 + 数字'
    return
  }
  if (!broker) {
    error.value = 'Broker 地址不能为空'
    return
  }

  saving.value = true
  try {
    if (editing.value) {
      await axios.put(`/api/docks/${encodeURIComponent(editing.value.sn)}`, {
        broker,
        username: form.value.username,
        password: form.value.password,
        orgId: form.value.orgId,
        bindingCode: form.value.bindingCode,
        droneSn: editing.value.droneSn,
      })
    } else {
      await axios.post('/api/docks', {
        sn,
        broker,
        username: form.value.username,
        password: form.value.password,
        orgId: form.value.orgId,
        bindingCode: form.value.bindingCode,
      })
    }
    dialogVisible.value = false
    await loadDocks()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '保存失败'
  } finally {
    saving.value = false
  }
}

async function remove(d: DockInfo) {
  if (!window.confirm(`确认删除机巢 ${d.sn}？`)) return
  try {
    await axios.delete(`/api/docks/${encodeURIComponent(d.sn)}`)
    await loadDocks()
  } catch (e: any) {
    window.alert(e?.response?.data?.error || '删除失败')
  }
}

async function reconnect(d: DockInfo) {
  try {
    await axios.post(`/api/docks/${encodeURIComponent(d.sn)}/reconnect`)
    await loadDocks()
  } catch (e: any) {
    window.alert(e?.response?.data?.error || '重连失败')
  }
}

onMounted(loadDocks)
</script>

<style scoped>
.dock-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.dock-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.dock-count {
  font-size: 12px;
  color: #8a8aaa;
}

.add-btn {
  padding: 6px 14px;
  border: 1px solid rgba(0, 255, 255, 0.4);
  border-radius: 6px;
  background: rgba(0, 255, 255, 0.1);
  color: #00ffff;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

.add-btn:hover {
  background: rgba(0, 255, 255, 0.2);
}

.empty {
  color: #555;
  font-size: 13px;
  text-align: center;
  padding: 16px 0;
}

.dock-item {
  background: #0f3460;
  border-radius: 6px;
  padding: 10px;
  border-left: 3px solid transparent;
}

.dock-item-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: #666;
}

.status-dot.online {
  background: #00ff88;
  box-shadow: 0 0 6px rgba(0, 255, 136, 0.6);
}

.status-dot.offline {
  background: #ff4444;
}

.dock-sn {
  flex: 1;
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 13px;
  color: #eee;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-text {
  font-size: 11px;
  flex-shrink: 0;
}

.status-text.online {
  color: #00ff88;
}

.status-text.offline {
  color: #ff6666;
}

.dock-broker {
  font-size: 11px;
  color: #8899bb;
  margin: 4px 0 8px 16px;
  font-family: 'Consolas', 'Courier New', monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dock-drone-sn {
  font-size: 11px;
  color: #66ccaa;
  margin: 0 0 8px 16px;
  font-family: 'Consolas', 'Courier New', monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dock-error {
  font-size: 11px;
  color: #ff6666;
  margin: 0 0 8px 16px;
  word-break: break-all;
}

.dock-actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}

.act-btn {
  padding: 4px 10px;
  border: 1px solid #2a2a4a;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.05);
  color: #8a8aaa;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.act-btn:hover {
  border-color: #00ffff;
  color: #00ffff;
}

.act-btn.danger:hover {
  border-color: #ff4444;
  color: #ff4444;
}

/* 模态框 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  width: 360px;
  max-width: 90vw;
  background: #1a1a2e;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 10px;
  padding: 20px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
}

.modal h3 {
  margin: 0 0 16px;
  font-size: 15px;
  color: #fff;
  letter-spacing: 1px;
}

.form-row {
  margin-bottom: 12px;
}

.form-row label {
  display: block;
  font-size: 12px;
  color: #8a8aaa;
  margin-bottom: 5px;
}

.form-row input {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.06);
  color: #eee;
  font-size: 13px;
  box-sizing: border-box;
}

.form-row input:focus {
  outline: none;
  border-color: rgba(0, 255, 255, 0.5);
}

.form-row input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.password-field {
  display: flex;
  align-items: center;
  gap: 6px;
}

.password-field input {
  flex: 1;
}

.toggle-eye {
  padding: 6px 10px;
  border: 1px solid #2a2a4a;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.05);
  color: #8a8aaa;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  transition: all 0.2s;
}

.toggle-eye:hover {
  border-color: #00ffff;
  color: #00ffff;
}

.error {
  color: #ff6666;
  font-size: 12px;
  margin: 4px 0 10px;
}

.modal-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  margin-top: 8px;
}

.cancel-btn,
.save-btn {
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  border: none;
}

.cancel-btn {
  background: #444;
  color: #ccc;
}

.cancel-btn:hover {
  background: #555;
}

.save-btn {
  background: #00ff88;
  color: #1a1a2e;
  font-weight: 600;
}

.save-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
