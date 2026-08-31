import { useEffect, useState, type ImgHTMLAttributes } from 'react'
import client, { API_BASE } from '@/api/client'
import { Spinner } from '@/components/ui'

interface AuthenticatedImageProps extends Omit<ImgHTMLAttributes<HTMLImageElement>, 'src'> {
  src: string
}

/**
 * 图片接口受登录态保护，不能直接用 <img src="/api/..."> 发送 Authorization。
 * 通过 axios 取 blob 后再交给 img，保证缩略图、原图切片和详情弹窗都能正常显示。
 */
export default function AuthenticatedImage({ src, alt, className = '', onError, ...props }: AuthenticatedImageProps) {
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')

  useEffect(() => {
    let disposed = false
    let nextObjectUrl: string | null = null

    setObjectUrl(null)
    setStatus('loading')

    const requestPath = src.startsWith(API_BASE) ? src.slice(API_BASE.length) || '/' : src

    client
      .get(requestPath, { responseType: 'blob' })
      .then((response) => {
        if (disposed) return
        nextObjectUrl = URL.createObjectURL(response.data)
        setObjectUrl(nextObjectUrl)
        setStatus('ready')
      })
      .catch(() => {
        if (!disposed) setStatus('error')
      })

    return () => {
      disposed = true
      if (nextObjectUrl) URL.revokeObjectURL(nextObjectUrl)
    }
  }, [src])

  if (status === 'ready' && objectUrl) {
    return (
      <img
        {...props}
        src={objectUrl}
        alt={alt}
        className={className}
        onError={(event) => {
          onError?.(event)
          setStatus('error')
        }}
      />
    )
  }

  return (
    <div
      role="img"
      aria-label={alt}
      className={`${className} flex min-h-16 items-center justify-center bg-surface-secondary/60 text-[10px] text-muted-foreground`}
    >
      {status === 'loading' ? <Spinner className="h-4 w-4 text-primary" /> : '图片暂时无法显示'}
    </div>
  )
}
