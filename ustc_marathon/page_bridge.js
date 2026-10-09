async ({path, params, timeout}) => {
  const permitted = new Set([
    '/item/scItem/queryApplyingMarathonItem', '/item/scItem/queryById',
    '/lib/scItemProgramme/selectEvaluationByItemId',
    '/item/scItem/marathon/myInfo', '/item/scItemRegistration/getOptions',
    '/item/scParticipateItem/list?queryCatelog=0'
  ]);
  if (!permitted.has(path) && !/^\/item\/scItem\/marathon\/my\/[A-Za-z0-9_-]+$/.test(path)) {
    throw new Error('查询路径未获得允许');
  }
  if (location.origin !== 'https://young.ustc.edu.cn') throw new Error('当前页面需要认证');
  const vm = document.querySelector('#app')?.__vue__;
  if (!vm?.$http?.get) throw new Error('网站查询机制发生变化');
  // 使用网站自己的 Axios 实例，保留原有参数加密和认证处理。
  return vm.$http.get(path, {params, timeout}).then(
    data => ({kind: 'response', data}),
    error => {
      const response = error.response;
      if (response && (response.status === 401 || response.data?.message === 'Token失效，请重新登录')) {
        return {kind: 'authentication'};
      }
      throw new Error('网站查询失败；请核查网络和服务器状态');
    }
  );
}
