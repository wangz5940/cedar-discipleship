<script setup>
const chapters = [
  {
    id: 'start', title: '开始使用', audience: '所有用户', image: '/assets/guide/personal-settings-current.png', imageAlt: '当前版本个人设置页面，展示本组名称、移动端卡片显示和修改密码', imageCaption: '个人设置：本组名称只影响自己在当前学习小组中的显示。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '登录与切换学习小组', steps: [
        '使用管理员提供的账号和密码登录。登录后先确认当前学习小组；属于多个小组时，可通过页面顶部的小组选择器切换。',
        '切换小组后，学习任务、成员、资料和统计都会随之切换。操作前再次核对当前小组。',
        '个人设置可修改自己在当前学习小组中的显示名称、移动端卡片显示方式和自己的密码；手机端从“更多”打开。修改密码后使用新密码重新登录。',
      ] },
      { title: '在哪里找到功能', steps: [
        '桌面侧栏的“今日”处理任务与打卡，“统计”查看完成情况，“资源”查找资料；手机底栏对应“学习”“统计”“资料”。专项小组入口按当前小组配置显示。',
        '有学习小组管理权限的用户还能看到“管理工作台”。各小组的专项小组入口可能因配置而隐藏。',
      ] },
    ],
  },
  {
    id: 'checkin', title: '学习任务与打卡', audience: '所有用户', image: '/assets/guide/today-learning-current.png', imageAlt: '当前版本今日学习页面，展示日期切换、任务卡片和打卡状态', imageCaption: '今日学习：从日期选择器查看当天或过去的任务，并在任务卡片中完成或取消打卡。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '完成当天任务', steps: [
        '进入“今日学习”，查看每日灵修、读经和本周的读物、音视频、背经或提纲任务。实际显示内容由当前小组配置决定。',
        '先打开任务内的阅读、观看或默写入口，完成学习后点击“完成并打卡”。卡片变为“已打卡”即表示记录成功。',
        '误点时可点击“取消打卡”。日期选择器可查看和补记过去日期；未来日期不能查看或打卡。',
      ] },
      { title: '灵修与读经怎样签到', steps: [
        '默认采用合并签到：灵修和读经共用一次“完成并打卡”。管理员开启“灵修与读经分别签到”后，两项各有自己的签到按钮和完成状态。',
        '如果管理员选择“按日自定义”灵修计划，未配置的日期不显示灵修内容；当天的读经是否显示，仍由读经配置决定。',
      ] },
      { title: '背经默写与任务资料', steps: [
        '背经任务点击“默写”进入默写页，再按页面提示填写和提交。任务卡片上的资料入口可打开对应读物、视频或图片。',
        '每日灵修等任务有多份资料时，卡片内会分别列出；一份资料时显示单独的阅读或观看按钮。音视频配套内容需进入播放器查看。',
      ] },
    ],
  },
  {
    id: 'media', title: '音视频与关联资料', audience: '所有用户', image: '/assets/guide/media-related-current-demo.png', imageAlt: '当前版本播放器组件，侧栏展示文字稿、配套讲义和音视频', imageCaption: '关联资料：桌面端从侧栏切换，手机端从上方“学习目录”展开。此图由当前版本播放器组件和演示资料生成。',
    topics: [
      { title: '播放与查看配套内容', steps: [
        '在“今日学习”的音视频任务点击“播放”打开主媒体。任务卡片只显示主媒体入口；匹配到的文字稿、配套读物或讲义会显示在播放器中。',
        '进入播放器后，桌面端从侧方的相关资料栏切换内容；手机端从播放器上方的“学习目录”展开查看。当前打开的资料会有状态提示。',
        '配套文字稿、逐字稿或讲义可以在同一标题前缀后加说明后缀，但资料须归入“读物”或“讲义”分类，且标题前缀与音视频相匹配。只改文件名或选错分类，都可能无法出现在播放器中。',
        '关联资料还须在当前小组可用。某项未出现时，请让管理员核对资源标题、分类和共享权限；每个音视频任务仍单独打卡。',
      ] },
    ],
  },
  {
    id: 'statistics', title: '查看学习统计', audience: '所有用户', image: '/assets/guide/statistics-current.png', imageAlt: '当前版本统计中心页面，展示日期、完成率和成员任务状态', imageCaption: '统计中心：上方查看所选日期的完成率，下方查看成员打卡明细。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '小组统计', steps: [
        '进入“统计”页，选择日期或统计范围，查看任务完成率、成员完成情况、分项统计和排行。',
        '可在成员矩阵中查看任务状态；只能操作自己的打卡。需要留存图表时使用页面提供的 PNG 导出。',
      ] },
      { title: '门训数点组的首页统计', steps: [
        '已加入“门训数点”专项小组的成员，在“今日学习”任务下方可看到按学习分项统计的完成数和柱状图，并可导出 PNG。',
        '这一区域只对当前学习小组内符合成员条件的账号显示；它展示学习打卡统计，与专项小组“数点组”的考勤表不同。',
      ] },
    ],
  },
  {
    id: 'resources', title: '查找与下载资料', audience: '所有用户', image: '/assets/guide/resources-current.png', imageAlt: '当前版本资源中心页面，展示搜索、分类筛选、查看和下载入口', imageCaption: '资源中心：先搜索或选择分类，再从资料卡片打开或下载。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '小组资料库', steps: [
        '进入“资源”页，按标题或文件名搜索，也可选择资料分类。点击资料标题或“查看”进行预览。',
        '点击“下载”保存单项资料；勾选多项后可批量下载。手机端可从“更多”进入“下载中心”查看任务。',
      ] },
    ],
  },
  {
    id: 'ministry', title: '专项小组协作', audience: '已开放专项小组的用户', image: '/assets/guide/ministry-admin-current.png', imageAlt: '当前版本管理工作台的专项小组目录设置', imageCaption: '管理工作台中的专项小组目录：管理员在此维护入口和小组名称。普通成员的小组页面按权限显示。',
    topics: [
      { title: '加入、分享与进展', steps: [
        '进入“专项小组”，查看已加入和可加入的小组。未加入时可提交申请；如小组要求审批，等待管理员处理。',
        '进入小组后可查看成员、经验分享和最近进展。具备相应权限时，可写分享或记录进展，并添加附件；需要审批的分享会先进入待处理状态。',
        '本人或管理员可在有权限时到“回收站”查看并恢复已删除的分享与进展。',
      ] },
      { title: '专项小组管理', steps: [
        '专项小组组长或管理员进入小组内“管理”，处理加入申请、分享审批、成员角色、可见范围及免审批设置。',
        '学习小组管理员可在“管理工作台 > 专项小组”维护本学习小组的专项小组目录；目录配置与小组内的日常管理是两个入口。',
      ] },
    ],
  },
  {
    id: 'counting', title: '数点组考勤', audience: '数点组成员及管理者', image: '/assets/guide/attendance-current-demo.png', imageAlt: '当前版本数点组考勤组件，展示月份、固定考勤日和考勤表', imageCaption: '数点组考勤：先选月份，再设置考勤日或记录出勤。此图由当前版本考勤组件和演示成员数据生成。',
    topics: [
      { title: '月度考勤表', steps: [
        '进入“专项小组”，打开“数点”小组，再选择“考勤”。使用月份选择器查看当月考勤表。',
        '有管理权限的用户可勾选固定考勤星期、添加临时考勤日期，并点击“保存设置”。有标记权限的用户点击成员与日期交叉处记录或取消出勤。',
        '可按姓名或出勤次数排序，也可导出当月 CSV。没有考勤日时，先由管理员设置日期。此处记录的是出勤，不会代替“今日学习”的任务打卡。',
      ] },
    ],
  },
  {
    id: 'learning-admin', title: '布置学习任务', audience: '学习小组管理者', image: '/assets/guide/weekly-plan-current.png', imageAlt: '当前版本周任务配置页面，展示周选择、日期与任务标题', imageCaption: '周任务配置：先选择目标周并核对起止日期，再向下填写各类任务并保存。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '每日灵修：连续计划与按日自定义', steps: [
        '进入“管理工作台 > 学习配置 > 每日学习配置”。“连续计划”选定一份灵修资源后自动安排：文字资源按文章标题定位，每天一篇；PDF 设置起始日期和起始页码后，每天一页。文字资源可按页面选项选择自动识别、篇号或日期标题定位。',
        '“按日自定义”可逐日填写日期、标题和内容。PDF 可为每一天指定开始页、结束页；界面也支持文字资源按日指定篇号。没有配置的日期不显示灵修内容。每天编辑后点击“保存当天计划”。',
        '“灵修与读经分别签到”默认关闭，开启后两项分别签到；保持关闭时合并签到。每日读经另选起始书卷、日期、章节和每日章数。保存学习配置后，到“今日学习”核对所选日期的实际内容。',
      ] },
      { title: '打卡通知', steps: [
        '“打卡通知”中的“每日灵修通知”和“周任务通知”分别控制对应的完成通知，开关默认开启；关闭某项后，该项打卡不会发送完成通知。实际发送还需要有可用的机器人和群聊绑定。',
      ] },
      { title: '每日背经', steps: [
        '在“学习配置 > 每日背经配置”开启显示，选择日期并填写带章节标记的默写原文，确认自动生成的标题后点击“保存当天背经”。可以从已配置日期中选择一天修改或删除。',
        '在“今日学习”打开原文或进行默写。',
      ] },
      { title: '新建或修改周任务', steps: [
        '在“学习配置 > 周任务”选择已有周，或点击“新增一周”；填写开始与结束日期、可选的自定义标题。',
        '启用需要的书籍、音视频、背经和提纲。读物选择资源及页码；音视频可逐项新增并选择文件；周背经填写经文与原文，整周完成一次；提纲选择图片。',
        '如果下周与上周安排同一份音视频资源，已经完成的状态会沿用到新一周，无须重复打卡；其他类型的周任务按各自任务记录完成状态。核对音视频资源是否确实为同一份文件。',
        '点击“保存当前周”后，到“今日学习”检查每项资料与打卡卡片。调整日期、恢复默认或删除当前周会影响成员看到的任务，操作前核对目标周。',
      ] },
    ],
  },
  {
    id: 'members-admin', title: '人员与权限', audience: '学习小组管理者', image: '/assets/guide/members-admin-current.png', imageAlt: '当前版本人员管理页面，展示小组设置、默认密码和添加人员入口', imageCaption: '人员管理：在当前小组内设置小组信息、默认密码和成员。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '管理当前小组成员', steps: [
        '进入“管理工作台 > 人员管理”，查看当前小组名单。具备对应权限时，可添加成员、设为或取消管理员、更新本组默认密码。',
        '修改本组默认密码时，系统会重置只属于本学习小组、且不是组长或超级管理员的账号密码；同时属于多个学习小组的成员不受影响，其现有密码不会随本组默认密码改变。',
        '创建或移除成员前核对当前小组和账号。已有账号加入小组时，按页面上的身份确认提示操作，避免重复建号。',
        '小家管理员或超级管理员还可在此创建、修改学习小组；不同角色看到的操作按钮会不同。',
      ] },
    ],
  },
  {
    id: 'resource-admin', title: '资源管理', audience: '学习小组管理者', image: '/assets/guide/resource-admin-current.png', imageAlt: '当前版本资源管理页面，展示上传分类和选择文件入口', imageCaption: '资源管理：先选择上传分类和文件，再上传到资源库，完成后在列表中核对。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '上传与共享', steps: [
        '进入“管理工作台 > 资源管理”，选好“上传到”的分类，可一次选择多个文件并上传。上传后可在周任务中挂载。',
        '本组上传的资源默认向同一小家的其他学习小组开放共享。可在“共享权限”改为仅本组、指定小组或所有小组；到“共享资源”中可导入同一小家其他小组开放的资料。',
        '来源小组修改自有资源的名称或分类时，仍在使用该资料的导入小组会同步看到改动；导入小组自己修改导入资源时，只影响本组。',
        '音视频的文字稿或讲义要归入“读物”或“讲义”，标题与音视频使用相同前缀，可在后面添加“文字稿”“讲义”等后缀；完成后打开播放器核对。',
        '删除正在被学习任务使用的资源会被阻止。需要替换时，先调整任务引用，再处理旧资源。',
      ] },
    ],
  },
  {
    id: 'data-admin', title: '数据与高级管理', audience: '具备对应权限的管理者', image: '/assets/guide/data-admin-current.png', imageAlt: '当前版本数据管理页面，展示导出与导入入口', imageCaption: '数据管理：导出和导入分列显示；导入文件前先核对当前小组及备份。截图取自当前本地预览的测试小组。',
    topics: [
      { title: '导出与导入', steps: [
        '“管理工作台 > 数据管理”可导出打卡明细、每日汇总、门训任务和本地备份。导出文件属于当前学习小组，请先核对小组。',
        '导入门训任务 Excel 会写入周任务；导入本地备份 JSON 会恢复当前小组的成员、学习和资源数据，不包含建议与反馈。导入前先下载备份，逐项确认文件和页面提示。',
      ] },
      { title: '按角色显示的功能', steps: [
        '“小家管理”可创建或选择小家、管理小家名称与所属学习小组，并维护小家管理员；具体按钮按角色显示。',
        '“机器人管理”可查看机器人状态、新增机器人和设置群聊与学习小组的绑定；只有获授权角色可见。',
        '“默写记录”可按成员查看历史默写、加载更多或删除记录；删除前核对成员和记录内容。',
        '这些栏目只对获授权角色开放。执行跨小组管理前，先核对目标小家、学习小组和页面上的操作提示。',
      ] },
    ],
  },
];
</script>

<template>
  <article class="user-guide">
    <header class="pagehead user-guide__header">
      <h1>使用文档</h1>
    </header>

    <div class="user-guide__layout">
      <nav class="card user-guide__toc" aria-label="文档目录">
        <h2>目录</h2>
        <a v-for="chapter in chapters" :key="chapter.id" :href="`#guide-${chapter.id}`">{{ chapter.title }}</a>
      </nav>

      <div class="user-guide__content">
        <section v-for="chapter in chapters" :id="`guide-${chapter.id}`" :key="chapter.id" class="card user-guide__chapter">
          <div class="user-guide__chapter-head">
            <h2>{{ chapter.title }}</h2>
            <span class="pill">{{ chapter.audience }}</span>
          </div>
          <div v-for="topic in chapter.topics" :key="topic.title" class="user-guide__topic">
            <h3>{{ topic.title }}</h3>
            <ol>
              <li v-for="step in topic.steps" :key="step">{{ step }}</li>
            </ol>
          </div>
          <figure v-if="chapter.image" class="user-guide__figure">
            <a :href="chapter.image" target="_blank" rel="noopener noreferrer" :aria-label="`放大查看：${chapter.imageAlt}`">
              <img :src="chapter.image" :alt="chapter.imageAlt" loading="lazy" />
            </a>
            <figcaption>{{ chapter.imageCaption }}</figcaption>
          </figure>
        </section>
      </div>
    </div>
  </article>
</template>

<style scoped>
.user-guide { min-width: 0; }
.user-guide__header { display: block; }
.user-guide__header p { margin-top: 8px; }
.user-guide__layout { display: grid; grid-template-columns: minmax(180px, 220px) minmax(0, 1fr); align-items: start; gap: 20px; }
.user-guide__toc { position: sticky; top: 20px; display: grid; gap: 4px; padding: 16px; }
.user-guide__toc h2 { margin-bottom: 6px; font-size: 16px; }
.user-guide__toc a { display: block; padding: 8px 10px; border-radius: 8px; color: var(--cd-text); text-decoration: none; }
.user-guide__toc a:hover, .user-guide__toc a:focus-visible { background: var(--cd-primary-soft); }
.user-guide__content { display: grid; gap: 16px; min-width: 0; }
.user-guide__chapter { scroll-margin-top: 24px; padding: 24px; }
.user-guide__chapter-head { display: flex; align-items: start; justify-content: space-between; gap: 12px; }
.user-guide__chapter-head h2 { font-size: 21px; }
.user-guide__chapter-head .pill { flex: 0 0 auto; }
.user-guide__topic + .user-guide__topic { margin-top: 24px; }
.user-guide__topic h3 { margin: 20px 0 10px; font-size: 17px; }
.user-guide__topic ol { display: grid; gap: 9px; margin: 0; padding-left: 1.5em; line-height: 1.75; }
.user-guide__topic li { padding-left: 3px; overflow-wrap: anywhere; }
.user-guide__figure { margin: 24px 0 0; }
.user-guide__figure img { display: block; width: 100%; height: auto; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); }
.user-guide__figure figcaption { margin-top: 8px; color: var(--cd-muted); font-size: 13px; line-height: 1.6; }
@media (max-width: 767px) {
  .user-guide__layout { grid-template-columns: minmax(0, 1fr); }
  .user-guide__toc { position: static; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .user-guide__toc h2 { grid-column: 1 / -1; }
  .user-guide__chapter { padding: 18px; }
  .user-guide__chapter-head { flex-wrap: wrap; }
}
</style>
