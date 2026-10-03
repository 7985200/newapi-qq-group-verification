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
import { CheckCircle2, Loader2 } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'

import { bindQQGroup, getQQGroupInfo } from '../api'
import type { QQGroupInfo } from '../types'

interface QQGroupBindDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

function formatCountdown(seconds: number): string {
  const safe = Math.max(0, seconds)
  const minutes = Math.floor(safe / 60)
  const rest = safe % 60
  return `${minutes}:${String(rest).padStart(2, '0')}`
}

export function QQGroupBindDialog({
  open,
  onOpenChange,
  onSuccess,
}: QQGroupBindDialogProps) {
  const [info, setInfo] = useState<QQGroupInfo | null>(null)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [code, setCode] = useState('')
  // 每秒走一格的“服务器当前秒”：用 server_time 抵消客户端时钟偏差
  const [nowSec, setNowSec] = useState(() => Math.floor(Date.now() / 1000))
  const clockOffsetRef = useRef(0)
  // 倒计时归零后自动刷新一次信息：用户可能已在群里重新获取了验证码
  const refreshedAfterExpiryRef = useRef(false)

  const loadInfo = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getQQGroupInfo()
      if (res.success && res.data) {
        setInfo(res.data)
        if (res.data.server_time) {
          clockOffsetRef.current = res.data.server_time * 1000 - Date.now()
        }
        refreshedAfterExpiryRef.current = false
      }
    } catch {
      // ignore, dialog falls back to defaults
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!open) return
    setCode('')
    setInfo(null)
    void loadInfo()
  }, [open, loadInfo])

  useEffect(() => {
    if (!open) return
    const timer = setInterval(() => {
      setNowSec(Math.floor((Date.now() + clockOffsetRef.current) / 1000))
    }, 1000)
    return () => clearInterval(timer)
  }, [open])

  const expiresAt = info?.code_expires_at
  const remaining = expiresAt != null ? Math.max(0, expiresAt - nowSec) : null
  const expired = expiresAt != null && remaining !== null && remaining <= 0
  const hasActiveCode = expiresAt != null && !expired
  const alreadyBound = (info?.qq_verified_at ?? 0) > 0

  useEffect(() => {
    if (!open || !expired || refreshedAfterExpiryRef.current) return
    refreshedAfterExpiryRef.current = true
    const timer = setTimeout(() => void loadInfo(), 1500)
    return () => clearTimeout(timer)
  }, [open, expired, loadInfo])

  const handleSubmit = async () => {
    if (submitting) return
    if (!/^\d{6}$/.test(code.trim())) {
      toast.error('请输入 6 位数字验证码')
      return
    }
    setSubmitting(true)
    try {
      const res = await bindQQGroup(code.trim())
      if (res.success) {
        toast.success('QQ 群验证完成！')
        onOpenChange(false)
        onSuccess?.()
      } else {
        toast.error(res.message || '验证失败')
      }
    } catch {
      toast.error('验证失败，请稍后再试')
    } finally {
      setSubmitting(false)
    }
  }

  const countdownLine = expired ? (
    <span className='text-destructive text-xs font-medium'>
      验证码已过期，请在群里重新获取
    </span>
  ) : hasActiveCode ? (
    <span className='text-muted-foreground font-mono text-xs tabular-nums'>
      有效期剩余 {formatCountdown(remaining ?? 0)}
    </span>
  ) : (
    <span className='text-muted-foreground text-xs'>
      还没有有效验证码，请在群里发送 /获取验证码
    </span>
  )

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title='绑定QQ群'
      description='加入官方QQ群并获取验证码后，在这里提交即可完成验证'
      contentClassName='sm:max-w-md'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={submitting || alreadyBound}>
            {submitting && <Loader2 className='animate-spin' />}
            确认绑定
          </Button>
        </>
      }
    >
      {loading ? (
        <div className='text-muted-foreground flex items-center justify-center gap-2 py-8 text-sm'>
          <Spinner />
          加载中...
        </div>
      ) : alreadyBound ? (
        <div className='flex flex-col items-center gap-2 py-6'>
          <CheckCircle2 className='h-10 w-10 text-green-500' />
          <p className='text-sm font-medium'>已完成QQ群验证</p>
          <p className='text-muted-foreground text-xs'>
            绑定的QQ号：{info?.qq_id || '未知'}
          </p>
        </div>
      ) : (
        <div className='space-y-4'>
          <div className='bg-muted/50 rounded-lg border px-3 py-2.5 text-sm'>
            {(info?.groups?.length ?? 0) > 0 ? (
              <p>
                请先加入QQ群：
                <span className='text-primary font-semibold'>
                  {info?.groups.join('、')}
                </span>
                ，在群内发送 <code className='font-mono'>/获取验证码</code>{' '}
                获取验证码。
              </p>
            ) : (
              <p>
                请先加入官方QQ群，在群内发送{' '}
                <code className='font-mono'>/获取验证码</code> 获取验证码。
              </p>
            )}
          </div>

          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-2'>
              <p className='text-sm font-medium'>验证码</p>
              {countdownLine}
            </div>
            <Input
              value={code}
              onChange={(e) =>
                setCode(e.target.value.replace(/\D/g, '').slice(0, 6))
              }
              onKeyDown={(e) => {
                if (e.key === 'Enter') void handleSubmit()
              }}
              placeholder='输入 6 位数字验证码'
              inputMode='numeric'
              autoFocus
            />
            <p className='text-muted-foreground text-xs'>
              验证码 10 分钟内有效、60 秒内只能获取一次；账号邮箱需为本人QQ邮箱（与QQ号一致），否则无法通过验证。
            </p>
          </div>
        </div>
      )}
    </Dialog>
  )
}
