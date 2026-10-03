import { expect, it } from 'vitest';
import { mediaAssetID, parseHandoutCues, handoutPageAtTime } from './mediaHandout';
it('使用稳定资产 ID 而非过期的签名 URL', () => {
  expect(mediaAssetID({sourceURL:'/api/assets/9/download',url:'https://temp/video.mp4'})).toBe(9);
  expect(mediaAssetID({url:'/api/assets/8/stream?signature=temporary'})).toBe(8);
  expect(mediaAssetID({url:'blob:http://localhost/preview'})).toBe(0);
});
it('页码时间点支持分秒与时分秒，并拒绝重复页码和无效时间', () => {
  expect(parseHandoutCues('1 00:00\n2 03:30\n3 1:10:00')).toEqual([{page:1,time:0},{page:2,time:210},{page:3,time:4200}]);
  for (const value of ['0 00:00','1 -1','1 00:00\n1 00:30','1 abc','abc 0','10001 00:00']) expect(()=>parseHandoutCues(value)).toThrow();
  expect(parseHandoutCues('')).toEqual([]);
});
it('根据真实播放位置同步页码，前后跳转与暂停位置一致', () => {
  const cues = [{page:3,time:100},{page:1,time:0},{page:2,time:50}];
  expect(handoutPageAtTime(cues,101)).toBe(3); expect(handoutPageAtTime(cues,50)).toBe(2);
  expect(handoutPageAtTime(cues,10)).toBe(1); expect(handoutPageAtTime([],10)).toBeNull();
});
