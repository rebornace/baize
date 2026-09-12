import QRCode from 'qrcode'

/** Encode payload text into a PNG data URL for <img src>. */
export async function qrDataUrlFromText(text: string, size = 180): Promise<string> {
  const payload = text.trim()
  if (!payload) {
    throw new Error('empty qr payload')
  }
  return QRCode.toDataURL(payload, {
    width: size,
    margin: 1,
    errorCorrectionLevel: 'M',
  })
}
