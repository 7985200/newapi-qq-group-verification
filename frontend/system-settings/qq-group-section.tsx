/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/password-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchRow,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

interface QQGroupSectionProps {
  defaultValues: {
    QQGroupVerificationEnabled: boolean
    QQGroupNumber: string
  }
}

interface QQVerificationUser {
  id: number
  username: string
  display_name: string
  role: number
  status: number
  email: string
  group: string
  qq_id: string
  qq_verified_at: number
  created_at: number
}

type VerifiedFilter = 'all' | 'yes' | 'no'

const PAGE_SIZE = 10

export function QQGroupSection({ defaultValues }: QQGroupSectionProps) {
  const updateOption = useUpdateOption()
  const baselineRef = useRef(defaultValues)

  const [enabled, setEnabled] = useState(defaultValues.QQGroupVerificationEnabled)
  const [groupNumber, setGroupNumber] = useState(defaultValues.QQGroupNumber || '')
  const [botSecret, setBotSecret] = useState('')
  const [saving, setSaving] = useState(false)

  // 验证状态管理面板
  const [keyword, setKeyword] = useState('')
  const [verifiedFilter, setVerifiedFilter] = useState<VerifiedFilter>('all')
  const [page, setPage] = useState(1)
  const [items, setItems] = useState<QQVerificationUser[]>([])
  const [total, setTotal] = useState(0)
  const [listLoading, setListLoading] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [unbindTarget, setUnbindTarget] = useState<QQVerificationUser | null>(null)
  const [unbinding, setUnbinding] = useState(false)

  useEffect(() => {
    baselineRef.current = defaultValues
    setEnabled(defaultValues.QQGroupVerificationEnabled)
    setGroupNumber(defaultValues.QQGroupNumber || '')
  }, [defaultValues])

  const loadList = useCallback(async () => {
    setListLoading(true)
    try {
      const params = new URLSearchParams({
        p: String(page),
        page_size: String(PAGE_SIZE),
        keyword,
        verified:
          verifiedFilter === 'yes' ? '1' : verifiedFilter === 'no' ? '0' : '-1',
      })
      const res = await api.get(`/api/qq-group/verifications?${params}`)
      if (res.data?.success && res.data.data) {
        setItems((res.data.data.items ?? []) as QQVerificationUser[])
        setTotal(Number(res.data.data.total ?? 0))
      }
    } catch {
      // ignore
    } finally {
      setListLoading(false)
    }
  }, [page, keyword, verifiedFilter])

  useEffect(() => {
    void loadList()
  }, [loadList, reloadKey])

  const onSave = async () => {
    if (saving) return
    const updates: Array<{ key: string; value: string }> = []
    if (enabled !== baselineRef.current.QQGroupVerificationEnabled) {
      updates.push({
        key: 'QQGroupVerificationEnabled',
        value: enabled ? 'true' : 'false',
      })
    }
    if (groupNumber.trim() !== (baselineRef.current.QQGroupNumber || '')) {
      updates.push({ key: 'QQGroupNumber', value: groupNumber.trim() })
    }
    // Bot 密钥不回显，留空表示不修改
    if (botSecret.trim() !== '') {
      updates.push({ key: 'QQBotSecret', value: botSecret.trim() })
    }
    if (updates.length === 0) {
      toast.info('没有需要保存的修改')
      return
    }
    setSaving(true)
    try {
      for (const update of updates) {
        const res = await updateOption.mutateAsync(update)
        if (!res?.success) {
          toast.error(res?.message || '保存失败')
          return
        }
      }
      baselineRef.current = {
        QQGroupVerificationEnabled: enabled,
        QQGroupNumber: groupNumber.trim(),
      }
      setBotSecret('')
      toast.success('QQ 群验证设置已保存')
    } finally {
      setSaving(false)
    }
  }

  const handleUnbind = async () => {
    if (!unbindTarget) return
    setUnbinding(true)
    try {
      const res = await api.post('/api/qq-group/unbind', {
        user_id: unbindTarget.id,
      })
      if (res.data?.success) {
        toast.success(`已解除用户 ${unbindTarget.username} 的QQ群绑定`)
        setReloadKey((k) => k + 1)
      } else {
        toast.error(res.data?.message || '解除绑定失败')
      }
    } catch {
      toast.error('解除绑定失败')
    } finally {
      setUnbinding(false)
      setUnbindTarget(null)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <SettingsSection title='QQ 群验证'>
      <SettingsForm onSubmit={(e) => e.preventDefault()}>
        <SettingsPageFormActions
          onSave={onSave}
          isSaving={saving}
          saveLabel='保存 QQ 群验证设置'
        />
        <SettingsSwitchRow>
          <SettingsSwitchContent>
            <div className='text-sm font-medium'>启用 QQ 群验证</div>
            <p className='text-muted-foreground text-xs'>
              开启后，未完成 QQ 群绑定的普通用户调用模型接口（/v1/*）会被拦截并提示先加群验证；管理员不受影响。
            </p>
          </SettingsSwitchContent>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </SettingsSwitchRow>

        <div className='space-y-1.5'>
          <div className='text-sm font-medium'>QQ 群号</div>
          <Input
            value={groupNumber}
            onChange={(e) => setGroupNumber(e.target.value)}
            placeholder='例如：123456789，多个群号用逗号分隔'
          />
          <p className='text-muted-foreground text-xs'>
            填写后会显示在未验证提示里，例如「请加入QQ群 123456789
            并完成验证」；支持多个群号（逗号分隔），提示为「请加入QQ群 A 或 B 并完成验证」。
          </p>
        </div>

        <div className='space-y-1.5'>
          <div className='text-sm font-medium'>Bot 密钥</div>
          <PasswordInput
            value={botSecret}
            onChange={(e) => setBotSecret(e.target.value)}
            placeholder='留空表示不修改'
            autoComplete='off'
          />
          <p className="text-muted-foreground text-xs">
            AstrBot
            插件调用验证码接口时使用此密钥（X-QQ-Bot-Secret
            请求头）。已保存的密钥不会回显。
          </p>
        </div>
      </SettingsForm>

      <div className='mt-6 border-t pt-4'>
        <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
          <div>
            <h3 className='text-sm font-semibold'>验证状态管理</h3>
            <p className='text-muted-foreground text-xs'>
              查看每个用户的QQ群验证状态，可解除绑定要求其重新验证。
            </p>
          </div>
          <div className='flex items-center gap-2'>
            <Input
              value={keyword}
              onChange={(e) => {
                setPage(1)
                setKeyword(e.target.value)
              }}
              placeholder='搜索用户名/邮箱/QQ号'
              className='h-8 w-52 text-sm'
            />
            <Select
              value={verifiedFilter}
              onValueChange={(value) => {
                setPage(1)
                setVerifiedFilter((value as VerifiedFilter) ?? 'all')
              }}
            >
              <SelectTrigger className='h-8 w-28 text-sm'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>全部</SelectItem>
                <SelectItem value='yes'>已验证</SelectItem>
                <SelectItem value='no'>未验证</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className='overflow-hidden rounded-lg border'>
          <table className='w-full text-sm'>
            <thead className='bg-muted/50'>
              <tr className='text-muted-foreground text-left text-xs'>
                <th className='px-3 py-2 font-medium'>ID</th>
                <th className='px-3 py-2 font-medium'>用户名</th>
                <th className='px-3 py-2 font-medium'>邮箱</th>
                <th className='px-3 py-2 font-medium'>QQ号</th>
                <th className='px-3 py-2 font-medium'>状态</th>
                <th className='px-3 py-2 font-medium'>验证时间</th>
                <th className='px-3 py-2 font-medium'>操作</th>
              </tr>
            </thead>
            <tbody>
              {listLoading ? (
                <tr>
                  <td colSpan={7} className='text-muted-foreground px-3 py-8 text-center'>
                    <span className='inline-flex items-center gap-2'>
                      <Spinner /> 加载中...
                    </span>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={7} className='text-muted-foreground px-3 py-8 text-center'>
                    暂无数据
                  </td>
                </tr>
              ) : (
                items.map((user) => {
                  const verified = user.qq_verified_at > 0
                  return (
                    <tr key={user.id} className='border-t'>
                      <td className='px-3 py-2 font-mono text-xs'>{user.id}</td>
                      <td className='max-w-32 truncate px-3 py-2' title={user.username}>
                        {user.username}
                      </td>
                      <td className='max-w-48 truncate px-3 py-2' title={user.email}>
                        {user.email || '-'}
                      </td>
                      <td className='px-3 py-2 font-mono text-xs'>
                        {user.qq_id || '-'}
                      </td>
                      <td className='px-3 py-2'>
                        <StatusBadge
                          label={verified ? '已验证' : '未验证'}
                          variant={verified ? 'success' : 'neutral'}
                          copyable={false}
                        />
                      </td>
                      <td className='text-muted-foreground px-3 py-2 text-xs'>
                        {verified
                          ? new Date(user.qq_verified_at * 1000).toLocaleString()
                          : '-'}
                      </td>
                      <td className='px-3 py-2'>
                        {verified ? (
                          <Button
                            variant='outline'
                            size='sm'
                            className='text-destructive h-7 px-2 text-xs'
                            onClick={() => setUnbindTarget(user)}
                          >
                            解除绑定
                          </Button>
                        ) : (
                          <span className='text-muted-foreground text-xs'>-</span>
                        )}
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>

        <div className='text-muted-foreground mt-2 flex items-center justify-between text-xs'>
          <span>共 {total} 人</span>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              className='h-7 px-2 text-xs'
              disabled={page <= 1 || listLoading}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              上一页
            </Button>
            <span className='tabular-nums'>
              {page} / {totalPages}
            </span>
            <Button
              variant='outline'
              size='sm'
              className='h-7 px-2 text-xs'
              disabled={page >= totalPages || listLoading}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            >
              下一页
            </Button>
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={!!unbindTarget}
        onOpenChange={(open) => !open && setUnbindTarget(null)}
        title='解除QQ群绑定'
        desc={`确定解除用户 ${unbindTarget?.username ?? ''}（QQ号 ${unbindTarget?.qq_id ?? ''}）的QQ群验证吗？解除后该用户调用模型需要重新加群验证。`}
        confirmText='解除绑定'
        destructive
        handleConfirm={handleUnbind}
        isLoading={unbinding}
      />
    </SettingsSection>
  )
}
