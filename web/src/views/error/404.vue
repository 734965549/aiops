<template>
  <div class="not-found">
    <a-result
      status="404"
      title="404"
      subtitle="页面未找到"
    >
      <template #extra>
        <a-button
          type="primary"
          @click="$router.replace('/')"
        >
          返回首页
        </a-button>
      </template>
    </a-result>
  </div>
</template>

<style scoped>
.not-found {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
