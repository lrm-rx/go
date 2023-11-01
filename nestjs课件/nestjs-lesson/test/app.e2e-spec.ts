// import { Test, TestingModule } from '@nestjs/testing';
// import { INestApplication } from '@nestjs/common';
// import * as request from 'supertest';
// import { AppModule } from '../src/app.module';
// import { setupApp } from '../src/setup';
// import { AppFactory } from './app.factory';
// import * as pactum from 'pactum';
import * as Spec from 'pactum/src/models/Spec';

describe('AppController (e2e)', () => {
  // let app: INestApplication;
  let spec;
  beforeEach(() => {
    // console.log('app', global.app);
    // pactum.request.setBaseUrl('http://localhost:3000');
    spec = global.pactum as Spec;
  });

  it('/ (GET)', () => {
    // baseURL + port
    // return request(app.getHttpServer())
    //   .get('/api/v1/auth')
    //   .expect(200)
    //   .expect('Hello World!');
    return spec
      .get('/api/v1/auth')
      .expectStatus(200)
      .expectBodyContains('Hello World!');
  });
});
