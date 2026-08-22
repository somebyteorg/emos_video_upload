export function stripVideoNameTags(name: string) {
  return name
    .replace(/\.[^.]+$/, '')
    .replace(/[\[\](){}]/g, ' ')
    .replace(/\bS\d{1,2}E\d{1,3}\b/gi, ' ')
    .replace(/\bS\d{1,2}\b/gi, ' ')
    .replace(/\bE\d{1,3}\b/gi, ' ')
    .replace(/\b(19|20)\d{2}\b/g, ' ')
    .replace(
      /\b(complete|2160p|1080p|720p|480p|x264|x265|hevc|h\.?264|h\.?265|av1|bluray|blu-ray|web[- ]?dl|webrip|hdr10\+?|hdr|dovi|dolby vision|10bit|8bit|aac|flac|dts|truehd|remux|proper|repack|国语|粤语|简繁|中英)\b/gi,
      ' ',
    )
    .replace(/[._-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}
