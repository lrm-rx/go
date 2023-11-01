import * as Spec from 'pactum/src/models/Spec';

describe('Auth登录认证 e2e测试', () => {
  let spec: Spec;
  beforeEach(() => {
    spec = global.spec as Spec;
  });

  // 注册用户
  it('注册用户', () => {
    const user = {
      username: 'toimc1',
      password: '123456',
    };
    return spec
      .post('/api/v1/auth/signup')
      .withBody(user)
      .expectStatus(201)
      .expectBodyContains(user.username)
      .expectJsonLike({
        id: 1,
        username: user.username,
        roles: [
          {
            id: 2,
            name: '普通用户',
          },
        ],
      });
  });
  // 注册新用户
  // 重复注册该用户
  it('重复注册该用户', async () => {
    const user = {
      username: 'toimc1',
      password: '123456',
    };

    await global.pactum.spec().post('/api/v1/auth/signup').withBody(user);

    return spec
      .post('/api/v1/auth/signup')
      .withBody(user)
      .expectStatus(403)
      .expectBodyContains('用户已存在');
  });

  // 注册用户传参异常 username password -> 长度，类型，空
  it('注册用户传参异常 username', () => {
    const user = {
      username: 'toimc',
      password: '123456',
    };
    return spec
      .post('/api/v1/auth/signup')
      .withBody(user)
      .expectStatus(400)
      .expectBodyContains('"用户名长度必须在6到20之间');
  });

  // todo 作业：自行完成username,password效验测试用例

  // 登录用户
  it('登录用户', async () => {
    const user = {
      username: 'toimc1',
      password: '123456',
    };

    await global.pactum.spec().post('/api/v1/auth/signup').withBody(user);

    return spec
      .post('/api/v1/auth/signin')
      .withBody(user)
      .expectStatus(201)
      .expectBodyContains('access_token');
  });

  // 登录用户传参异常 username password -> 长度，类型，空
  it('登录用户传参异常 username', async () => {
    const user = {
      username: 'toimc1',
      password: '123456',
    };

    await global.pactum.spec().post('/api/v1/auth/signup').withBody(user);

    return spec
      .post('/api/v1/auth/signin')
      .withBody({ username: 'toimc' })
      .expectStatus(400)
      .expectBodyContains('用户名长度必须在6到20之间，当前传递的值是：toimc');
  });

  // 登录用户不存在
  it('登录用户不存在', async () => {
    const user = {
      username: 'toimc1',
      password: '123456',
    };

    // await global.pactum.spec().post('/api/v1/auth/signup').withBody(user);

    return spec
      .post('/api/v1/auth/signin')
      .withBody(user)
      .expectStatus(403)
      .expectBodyContains('用户不存在，请注册');
  });

  // 登录用户密码错误
  it('登录用户密码错误', async () => {
    const user = {
      username: 'toimc1',
      password: '123456',
    };

    await global.pactum.spec().post('/api/v1/auth/signup').withBody(user);

    return spec
      .post('/api/v1/auth/signin')
      .withBody({ ...user, password: '1234567' })
      .expectStatus(403)
      .expectBodyContains('用户名或者密码错误');
  });

  // 补充说明的：
  // user模块 -> headers -> token信息 -> beforeEach -> 获取token
});
